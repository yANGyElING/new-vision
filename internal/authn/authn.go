package authn

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/new-vision-lab/new-vision/internal/identity"
	"golang.org/x/crypto/bcrypt"
)

// Principal is the authenticated caller attached to the request context.
type Principal struct {
	UserID     string
	TenantID   string
	Username   string
	Roles      []string
	RegionIDs  []string
	// DelegatedIdentity is reserved for future Center-granted delegation.
	// It is never populated by local JWT login in this phase.
	DelegatedIdentity *DelegatedIdentity
}

// DelegatedIdentity is the extension point for Center operations that carry
// a delegated authority. Not consumed in the current phase.
type DelegatedIdentity struct {
	CenterID        string
	CenterUserID    string
	RequestedAction string
	TargetTenant    string
	TargetResource  string
	OperationID     string
	IssuedAt        time.Time
	ExpiresAt       time.Time
}

type contextKey struct{}

func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, p)
}

func PrincipalFrom(ctx context.Context) *Principal {
	p, _ := ctx.Value(contextKey{}).(*Principal)
	return p
}

// PasswordHasher wraps bcrypt for hash/verify.
type PasswordHasher struct{ cost int }

func NewPasswordHasher() *PasswordHasher { return &PasswordHasher{cost: bcrypt.DefaultCost} }

func (h *PasswordHasher) Hash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	return string(hash), err
}

func (h *PasswordHasher) Verify(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

type claims struct {
	TenantID string   `json:"tid"`
	Username string   `json:"usr"`
	Roles    []string `json:"rol"`
	jwt.RegisteredClaims
}

type TokenManager struct {
	secret []byte
	ttl    time.Duration
	hasher *PasswordHasher
}

func NewTokenManager(secret string, ttl time.Duration) (*TokenManager, error) {
	if len(secret) < 32 {
		return nil, errors.New("JWT secret must be at least 32 bytes")
	}
	if ttl <= 0 {
		return nil, errors.New("JWT TTL must be positive")
	}
	return &TokenManager{secret: []byte(secret), ttl: ttl, hasher: NewPasswordHasher()}, nil
}

func (m *TokenManager) Hasher() *PasswordHasher { return m.hasher }

func (m *TokenManager) Issue(u identity.User) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(m.ttl)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		TenantID: u.TenantID,
		Username: u.Username,
		Roles:    u.Roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			ID:        fmt.Sprintf("%d-%s", now.UnixNano(), u.ID),
		},
	})
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

func (m *TokenManager) Parse(tokenString string) (*Principal, error) {
	token, err := jwt.ParseWithClaims(tokenString, &claims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	c, ok := token.Claims.(*claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}
	return &Principal{
		UserID:   c.Subject,
		TenantID: c.TenantID,
		Username: c.Username,
		Roles:    c.Roles,
	}, nil
}
