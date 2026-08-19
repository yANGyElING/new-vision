/*
 * GB28181 Access control plane for Kamailio 6.1.3.
 *
 * Kamailio's jsonrpcs transport cannot scan nested JSON objects or arrays in
 * 6.1.3. This module therefore owns the /rpc JSON-RPC envelope while xhttp
 * remains responsible for HTTP transport.
 */
#include <ctype.h>
#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/time.h>
#include <time.h>
#include <unistd.h>

#include <hiredis/hiredis.h>
#include <jansson.h>
#include <libxml/parser.h>
#include <libxml/tree.h>

#include "../../core/dprint.h"
#include "../../core/parser/msg_parser.h"
#include "../../core/parser/parse_content.h"
#include "../../core/parser/parse_expires.h"
#include "../../core/parser/parse_from.h"
#include "../../core/parser/parse_uri.h"
#include "../../core/sr_module.h"
#include "../../core/usr_avp.h"
#include "../../core/timer.h"
#include "../xhttp/api.h"

MODULE_VERSION

#define RPC_BODY_MAX (16 * 1024 * 1024)
#define MAX_PROFILES 10000
#define CAS_ATTEMPTS 5

#define KEY_PREFIX "nv:access:v1:"
#define KEY_INSTANCE KEY_PREFIX "instance-id"
#define KEY_EPOCH KEY_PREFIX "session-epoch"
#define KEY_GENERATION KEY_PREFIX "active-generation"
#define KEY_PROFILE_IDS KEY_PREFIX "profile-ids"
#define KEY_PROFILE_REVISION KEY_PREFIX "profile-revision"
#define KEY_REGISTRATION_IDS KEY_PREFIX "registration-ids"
#define KEY_EVENT_SEQUENCE KEY_PREFIX "event-sequence"
#define KEY_EVENTS KEY_PREFIX "events"
#define KEY_ACKED KEY_PREFIX "acked:node-app"

#define DATA_INVALID_PROFILE "INVALID_PROFILE"
#define DATA_PROFILE_CONFLICT "PROFILE_VERSION_CONFLICT"
#define DATA_REDIS_UNAVAILABLE "REDIS_UNAVAILABLE"
#define DATA_INVALID_CURSOR "INVALID_CURSOR"
#define DATA_INTERNAL_ERROR "INTERNAL_ERROR"

static redisContext *redis_ctx;
static xhttp_api_t xhttp_api;
static char access_instance_id[65];
static char session_epoch[37];

typedef struct access_profile access_profile_t;

static int mod_init(void);
static int child_init(int rank);
static void mod_destroy(void);
static int w_ready(struct sip_msg *msg, char *p1, char *p2);
static int w_rpc_dispatch(struct sip_msg *msg, char *p1, char *p2);
static int w_authorize_register(struct sip_msg *msg, char *p1, char *p2);
static int w_record_registration(struct sip_msg *msg, char *p1, char *p2);
static int w_handle_keepalive(struct sip_msg *msg, char *p1, char *p2);
static int load_request_profile(struct sip_msg *msg, char device_id[21],
        access_profile_t *profile);
static int parse_register_expires(struct sip_msg *msg, unsigned int *expires);
static int runtime_register(struct sip_msg *msg, const char *device_id,
        unsigned int expires);
static int runtime_unregister(struct sip_msg *msg, const char *device_id,
        const char *reason);
static int parse_keepalive_body(struct sip_msg *msg, const char *device_id);
static long long unix_now(void);
static int keepalive_timeout(void);
static void access_timer(unsigned int ticks, void *param);

struct access_profile {
    char device_access_id[21];
    char sip_username[21];
    char sip_realm[256];
    char digest_ha1[33];
    long long version;
    int enabled;
    int tombstone;
    int exists;
};

static int valid_instance_id(const char *value)
{
    size_t i, n;

    if(!value)
        return 0;
    n = strlen(value);
    if(n == 0 || n >= sizeof(access_instance_id))
        return 0;
    for(i = 0; i < n; i++) {
        if(!isalnum((unsigned char)value[i]) && value[i] != '-'
                && value[i] != '_')
            return 0;
    }
    return 1;
}

static int valid_uuid(const char *value)
{
    size_t i;

    if(!value || strlen(value) != 36)
        return 0;
    for(i = 0; i < 36; i++) {
        if(i == 8 || i == 13 || i == 18 || i == 23) {
            if(value[i] != '-')
                return 0;
        } else if(!isxdigit((unsigned char)value[i])) {
            return 0;
        }
    }
    return 1;
}

static int generate_uuid(char output[37])
{
    unsigned char bytes[16];
    static const char hex[] = "0123456789abcdef";
    int fd;
    size_t i, at = 0;
    ssize_t got;

    fd = open("/dev/urandom", O_RDONLY);
    if(fd < 0)
        return -1;
    got = read(fd, bytes, sizeof(bytes));
    close(fd);
    if(got != (ssize_t)sizeof(bytes))
        return -1;
    bytes[6] = (unsigned char)((bytes[6] & 0x0f) | 0x40);
    bytes[8] = (unsigned char)((bytes[8] & 0x3f) | 0x80);
    for(i = 0; i < sizeof(bytes); i++) {
        if(i == 4 || i == 6 || i == 8 || i == 10)
            output[at++] = '-';
        output[at++] = hex[bytes[i] >> 4];
        output[at++] = hex[bytes[i] & 0x0f];
    }
    output[at] = '\0';
    return 0;
}

static void redis_close(void)
{
    if(redis_ctx) {
        redisFree(redis_ctx);
        redis_ctx = NULL;
    }
}

static int redis_connect(void)
{
    const char *host = getenv("NV_REDIS_HOST");
    const char *port_text = getenv("NV_REDIS_PORT");
    const char *username = getenv("NV_REDIS_USERNAME");
    const char *password = getenv("NV_REDIS_PASSWORD");
    struct timeval timeout = {1, 0};
    redisReply *reply;
    int port;

    if(redis_ctx && redis_ctx->err == 0)
        return 0;
    redis_close();
    port = port_text ? atoi(port_text) : 6379;
    if(!host || !username || !password || port < 1 || port > 65535)
        return -1;
    redis_ctx = redisConnectWithTimeout(host, port, timeout);
    if(!redis_ctx || redis_ctx->err)
        goto fail;
    if(redisSetTimeout(redis_ctx, timeout) != REDIS_OK)
        goto fail;
    reply = redisCommand(redis_ctx, "AUTH %b %b", username, strlen(username),
            password, strlen(password));
    if(!reply || reply->type == REDIS_REPLY_ERROR) {
        if(reply)
            freeReplyObject(reply);
        goto fail;
    }
    freeReplyObject(reply);
    return 0;

fail:
    redis_close();
    return -1;
}

static int reply_status_ok(redisReply *reply)
{
    return reply && (reply->type == REDIS_REPLY_STATUS
            || reply->type == REDIS_REPLY_INTEGER
            || reply->type == REDIS_REPLY_STRING);
}

static int redis_simple(const char *command)
{
    redisReply *reply;
    int ok;

    if(redis_connect() < 0)
        return -1;
    reply = redisCommand(redis_ctx, command);
    ok = reply_status_ok(reply);
    if(reply)
        freeReplyObject(reply);
    if(!ok && redis_ctx && redis_ctx->err)
        redis_close();
    return ok ? 0 : -1;
}

static int redis_ping(void)
{
    redisReply *reply;
    int ok;

    if(redis_connect() < 0)
        return -1;
    reply = redisCommand(redis_ctx, "PING");
    ok = reply && reply->type == REDIS_REPLY_STATUS && reply->str
            && strcmp(reply->str, "PONG") == 0;
    if(reply)
        freeReplyObject(reply);
    return ok ? 1 : -1;
}

static int queue_ok(redisReply *reply)
{
    int ok = reply && reply->type == REDIS_REPLY_STATUS && reply->str
            && strcmp(reply->str, "QUEUED") == 0;
    if(reply)
        freeReplyObject(reply);
    return ok ? 0 : -1;
}

/* 1 committed, 0 aborted by WATCH, -1 backend/command error. */
static int exec_transaction(void)
{
    redisReply *reply;
    size_t i;

    reply = redisCommand(redis_ctx, "EXEC");
    if(!reply)
        return -1;
    if(reply->type == REDIS_REPLY_NIL) {
        freeReplyObject(reply);
        return 0;
    }
    if(reply->type != REDIS_REPLY_ARRAY) {
        freeReplyObject(reply);
        return -1;
    }
    for(i = 0; i < reply->elements; i++) {
        if(!reply->element[i] || reply->element[i]->type == REDIS_REPLY_ERROR) {
            freeReplyObject(reply);
            return -1;
        }
    }
    freeReplyObject(reply);
    return 1;
}

static void unwatch(void)
{
    redisReply *reply = redisCommand(redis_ctx, "UNWATCH");
    if(reply)
        freeReplyObject(reply);
}

static void discard_transaction(void)
{
    redisReply *reply = redisCommand(redis_ctx, "DISCARD");
    if(reply)
        freeReplyObject(reply);
}

static int redis_get_ll(const char *key, long long *value)
{
    redisReply *reply;
    char *end;
    long long parsed;

    reply = redisCommand(redis_ctx, "GET %s", key);
    if(!reply)
        return -1;
    if(reply->type == REDIS_REPLY_NIL) {
        *value = 0;
        freeReplyObject(reply);
        return 0;
    }
    if(reply->type != REDIS_REPLY_STRING || !reply->str) {
        freeReplyObject(reply);
        return -1;
    }
    errno = 0;
    parsed = strtoll(reply->str, &end, 10);
    if(errno || *end != '\0' || parsed < 0) {
        freeReplyObject(reply);
        return -1;
    }
    *value = parsed;
    freeReplyObject(reply);
    return 0;
}

static void profile_key(const char *device_id, char output[64])
{
    snprintf(output, 64, KEY_PREFIX "profile:%s", device_id);
}

static void registration_key(const char *device_id, char output[72])
{
    snprintf(output, 72, KEY_PREFIX "registration:%s", device_id);
}

static int parse_bool_text(const char *value, int *result)
{
    if(strcmp(value, "0") == 0) {
        *result = 0;
        return 0;
    }
    if(strcmp(value, "1") == 0) {
        *result = 1;
        return 0;
    }
    return -1;
}

static int load_profile(const char *device_id, access_profile_t *profile)
{
    char key[64];
    redisReply *reply;
    size_t i;
    char *end;

    memset(profile, 0, sizeof(*profile));
    profile_key(device_id, key);
    reply = redisCommand(redis_ctx, "HGETALL %s", key);
    if(!reply)
        return -1;
    if(reply->type != REDIS_REPLY_ARRAY || reply->elements % 2 != 0) {
        freeReplyObject(reply);
        return -1;
    }
    if(reply->elements == 0) {
        freeReplyObject(reply);
        return 0;
    }
    profile->exists = 1;
    for(i = 0; i < reply->elements; i += 2) {
        const char *field = reply->element[i]->str;
        const char *value = reply->element[i + 1]->str;
        if(!field || !value)
            goto invalid;
        if(strcmp(field, "device_access_id") == 0) {
            if(strlen(value) != 20)
                goto invalid;
            memcpy(profile->device_access_id, value, 21);
        } else if(strcmp(field, "sip_username") == 0) {
            if(strlen(value) != 20)
                goto invalid;
            memcpy(profile->sip_username, value, 21);
        } else if(strcmp(field, "sip_realm") == 0) {
            if(strlen(value) >= sizeof(profile->sip_realm))
                goto invalid;
            strcpy(profile->sip_realm, value);
        } else if(strcmp(field, "digest_ha1") == 0) {
            if(strlen(value) != 32)
                goto invalid;
            memcpy(profile->digest_ha1, value, 33);
        } else if(strcmp(field, "version") == 0) {
            errno = 0;
            profile->version = strtoll(value, &end, 10);
            if(errno || *end != '\0' || profile->version <= 0)
                goto invalid;
        } else if(strcmp(field, "enabled") == 0) {
            if(parse_bool_text(value, &profile->enabled) < 0)
                goto invalid;
        } else if(strcmp(field, "tombstone") == 0) {
            if(parse_bool_text(value, &profile->tombstone) < 0)
                goto invalid;
        }
    }
    freeReplyObject(reply);
    return 0;

invalid:
    freeReplyObject(reply);
    return -1;
}

static int same_profile(const access_profile_t *left,
        const access_profile_t *right)
{
    return !left->tombstone && !right->tombstone
            && left->version == right->version
            && left->enabled == right->enabled
            && strcmp(left->device_access_id, right->device_access_id) == 0
            && strcmp(left->sip_username, right->sip_username) == 0
            && strcmp(left->sip_realm, right->sip_realm) == 0
            && strcmp(left->digest_ha1, right->digest_ha1) == 0;
}

static int object_has_only(json_t *object, const char *const *allowed,
        size_t allowed_count)
{
    const char *key;
    json_t *value;
    size_t i;
    int found;

    if(!json_is_object(object))
        return 0;
    json_object_foreach(object, key, value) {
        (void)value;
        found = 0;
        for(i = 0; i < allowed_count; i++) {
            if(strcmp(key, allowed[i]) == 0) {
                found = 1;
                break;
            }
        }
        if(!found)
            return 0;
    }
    return 1;
}

static int valid_device_id(const char *value)
{
    size_t i;

    if(!value || strlen(value) != 20)
        return 0;
    for(i = 0; i < 20; i++) {
        if(!isdigit((unsigned char)value[i]))
            return 0;
    }
    return 1;
}

static int valid_ha1(const char *value)
{
    size_t i;

    if(!value || strlen(value) != 32)
        return 0;
    for(i = 0; i < 32; i++) {
        if(!isdigit((unsigned char)value[i])
                && (value[i] < 'a' || value[i] > 'f'))
            return 0;
    }
    return 1;
}

static int valid_realm(const char *value)
{
    size_t i, n;

    if(!value)
        return 0;
    n = strlen(value);
    if(n == 0 || n >= 256 || isspace((unsigned char)value[0])
            || isspace((unsigned char)value[n - 1]))
        return 0;
    for(i = 0; i < n; i++) {
        if((unsigned char)value[i] < 0x20 || value[i] == 0x7f)
            return 0;
    }
    return 1;
}

static int parse_profile(json_t *value, access_profile_t *profile)
{
    static const char *const fields[] = {"device_access_id", "sip_username",
        "sip_realm", "digest_algorithm", "digest_ha1", "enabled", "version"};
    json_t *item;
    const char *device_id, *username, *realm, *algorithm, *ha1;
    json_int_t version;

    if(!object_has_only(value, fields, sizeof(fields) / sizeof(fields[0])))
        return -1;
    item = json_object_get(value, "device_access_id");
    if(!json_is_string(item))
        return -1;
    device_id = json_string_value(item);
    item = json_object_get(value, "sip_username");
    if(!json_is_string(item))
        return -1;
    username = json_string_value(item);
    item = json_object_get(value, "sip_realm");
    if(!json_is_string(item))
        return -1;
    realm = json_string_value(item);
    item = json_object_get(value, "digest_algorithm");
    if(!json_is_string(item))
        return -1;
    algorithm = json_string_value(item);
    item = json_object_get(value, "digest_ha1");
    if(!json_is_string(item))
        return -1;
    ha1 = json_string_value(item);
    item = json_object_get(value, "enabled");
    if(!json_is_boolean(item))
        return -1;
    profile->enabled = json_is_true(item);
    item = json_object_get(value, "version");
    if(!json_is_integer(item))
        return -1;
    version = json_integer_value(item);
    if(!valid_device_id(device_id) || strcmp(device_id, username) != 0
            || !valid_realm(realm) || strcmp(algorithm, "MD5") != 0
            || !valid_ha1(ha1) || version <= 0 || version > LLONG_MAX)
        return -1;
    memset(profile, 0, sizeof(*profile));
    strcpy(profile->device_access_id, device_id);
    strcpy(profile->sip_username, username);
    strcpy(profile->sip_realm, realm);
    strcpy(profile->digest_ha1, ha1);
    profile->enabled = json_is_true(json_object_get(value, "enabled"));
    profile->version = (long long)version;
    profile->exists = 1;
    return 0;
}

static json_t *profile_result(const char *status, long long version)
{
    return json_pack("{s:s,s:I}", "status", status, "version",
            (json_int_t)version);
}

static int send_json(struct sip_msg *msg, json_t *envelope)
{
    str reason = str_init("OK");
    str content_type = str_init("application/json");
    str body;
    char *encoded;
    int result;

    encoded = json_dumps(envelope, JSON_COMPACT | JSON_ENSURE_ASCII);
    if(!encoded)
        return -1;
    body.s = encoded;
    body.len = (int)strlen(encoded);
    result = xhttp_api.reply(msg, 200, &reason, &content_type, &body);
    free(encoded);
    return result;
}

static int send_result(struct sip_msg *msg, json_t *id, json_t *result)
{
    json_t *envelope = json_object();
    int sent;

    if(!envelope || !result)
        goto fail;
    if(json_object_set_new(envelope, "jsonrpc", json_string("2.0")) < 0
            || json_object_set(envelope, "id", id) < 0)
        goto fail;
    if(json_object_set(envelope, "result", result) < 0)
        goto fail;
    json_decref(result);
    result = NULL;
    sent = send_json(msg, envelope);
    json_decref(envelope);
    return sent;

fail:
    if(result)
        json_decref(result);
    if(envelope)
        json_decref(envelope);
    return -1;
}

static int send_error(struct sip_msg *msg, json_t *id, int code,
        const char *message, const char *data_code)
{
    json_t *envelope = json_object();
    json_t *error = json_object();
    json_t *data = NULL;
    int sent;

    if(!envelope || !error)
        goto fail;
    if(data_code) {
        data = json_pack("{s:s}", "code", data_code);
        if(!data || json_object_set_new(error, "data", data) < 0)
            goto fail;
        data = NULL;
    }
    if(json_object_set_new(error, "code", json_integer(code)) < 0
            || json_object_set_new(error, "message", json_string(message)) < 0
            || json_object_set_new(envelope, "jsonrpc", json_string("2.0")) < 0
            || (id ? json_object_set(envelope, "id", id)
                   : json_object_set_new(envelope, "id", json_null())) < 0
            || json_object_set_new(envelope, "error", error) < 0)
        goto fail;
    error = NULL;
    sent = send_json(msg, envelope);
    json_decref(envelope);
    return sent;

fail:
    if(data)
        json_decref(data);
    if(error)
        json_decref(error);
    if(envelope)
        json_decref(envelope);
    return -1;
}

static int required_request_id(json_t *params)
{
    json_t *value = json_object_get(params, "request_id");
    return json_is_string(value) && valid_uuid(json_string_value(value));
}

static int queue_profile(const access_profile_t *profile)
{
    char key[64];
    redisReply *reply;

    profile_key(profile->device_access_id, key);
    reply = redisCommand(redis_ctx,
            "HSET %s device_access_id %s sip_username %s sip_realm %b "
            "digest_algorithm MD5 digest_ha1 %s enabled %d version %lld tombstone 0",
            key, profile->device_access_id, profile->sip_username,
            profile->sip_realm, strlen(profile->sip_realm), profile->digest_ha1,
            profile->enabled, profile->version);
    return queue_ok(reply);
}

static int queue_clear_registration(const char *device_id)
{
    char key[72];
    redisReply *reply;

    registration_key(device_id, key);
    reply = redisCommand(redis_ctx, "DEL %s", key);
    if(queue_ok(reply) < 0)
        return -1;
    reply = redisCommand(redis_ctx, "SREM %s %s", KEY_REGISTRATION_IDS,
            device_id);
    return queue_ok(reply);
}

static int apply_profile(const access_profile_t *incoming, json_t **result,
        const char **error_code)
{
    access_profile_t stored;
    char key[64];
    redisReply *reply;
    int attempt, committed;

    profile_key(incoming->device_access_id, key);
    for(attempt = 0; attempt < CAS_ATTEMPTS; attempt++) {
        reply = redisCommand(redis_ctx, "WATCH %s", key);
        if(!reply_status_ok(reply)) {
            if(reply)
                freeReplyObject(reply);
            *error_code = DATA_REDIS_UNAVAILABLE;
            return -1;
        }
        freeReplyObject(reply);
        if(load_profile(incoming->device_access_id, &stored) < 0) {
            unwatch();
            *error_code = DATA_REDIS_UNAVAILABLE;
            return -1;
        }
        if(stored.exists && incoming->version < stored.version) {
            unwatch();
            *result = profile_result("stale", stored.version);
            return *result ? 0 : -1;
        }
        if(stored.exists && incoming->version == stored.version) {
            if(same_profile(incoming, &stored)) {
                unwatch();
                *result = profile_result("unchanged", stored.version);
                return *result ? 0 : -1;
            }
            unwatch();
            *error_code = DATA_PROFILE_CONFLICT;
            return -1;
        }
        if(redis_simple("MULTI") < 0 || queue_profile(incoming) < 0) {
            unwatch();
            *error_code = DATA_REDIS_UNAVAILABLE;
            return -1;
        }
        reply = redisCommand(redis_ctx, "SADD %s %s", KEY_PROFILE_IDS,
                incoming->device_access_id);
        if(queue_ok(reply) < 0)
            goto transaction_error;
        reply = redisCommand(redis_ctx, "INCR %s", KEY_PROFILE_REVISION);
        if(queue_ok(reply) < 0)
            goto transaction_error;
        if(!incoming->enabled
                && queue_clear_registration(incoming->device_access_id) < 0)
            goto transaction_error;
        committed = exec_transaction();
        if(committed < 0) {
            *error_code = DATA_REDIS_UNAVAILABLE;
            return -1;
        }
        if(committed == 0)
            continue;
        *result = profile_result("applied", incoming->version);
        return *result ? 0 : -1;

transaction_error:
        discard_transaction();
        *error_code = DATA_REDIS_UNAVAILABLE;
        return -1;
    }
    *error_code = DATA_INTERNAL_ERROR;
    return -1;
}

static int remove_profile(const char *device_id, long long version,
        json_t **result, const char **error_code)
{
    access_profile_t stored;
    char key[64];
    redisReply *reply;
    int attempt, committed;

    profile_key(device_id, key);
    for(attempt = 0; attempt < CAS_ATTEMPTS; attempt++) {
        reply = redisCommand(redis_ctx, "WATCH %s", key);
        if(!reply_status_ok(reply)) {
            if(reply)
                freeReplyObject(reply);
            *error_code = DATA_REDIS_UNAVAILABLE;
            return -1;
        }
        freeReplyObject(reply);
        if(load_profile(device_id, &stored) < 0) {
            unwatch();
            *error_code = DATA_REDIS_UNAVAILABLE;
            return -1;
        }
        if(stored.exists && version < stored.version) {
            unwatch();
            *result = profile_result("stale", stored.version);
            return *result ? 0 : -1;
        }
        if(stored.exists && version == stored.version) {
            if(stored.tombstone) {
                unwatch();
                *result = profile_result("unchanged", stored.version);
                return *result ? 0 : -1;
            }
            unwatch();
            *error_code = DATA_PROFILE_CONFLICT;
            return -1;
        }
        if(redis_simple("MULTI") < 0) {
            unwatch();
            *error_code = DATA_REDIS_UNAVAILABLE;
            return -1;
        }
        reply = redisCommand(redis_ctx, "DEL %s", key);
        if(queue_ok(reply) < 0)
            goto remove_error;
        reply = redisCommand(redis_ctx,
                "HSET %s device_access_id %s enabled 0 version %lld tombstone 1",
                key, device_id, version);
        if(queue_ok(reply) < 0)
            goto remove_error;
        reply = redisCommand(redis_ctx, "SADD %s %s", KEY_PROFILE_IDS,
                device_id);
        if(queue_ok(reply) < 0)
            goto remove_error;
        reply = redisCommand(redis_ctx, "INCR %s", KEY_PROFILE_REVISION);
        if(queue_ok(reply) < 0 || queue_clear_registration(device_id) < 0)
            goto remove_error;
        committed = exec_transaction();
        if(committed < 0) {
            *error_code = DATA_REDIS_UNAVAILABLE;
            return -1;
        }
        if(committed == 0)
            continue;
        *result = profile_result("applied", version);
        return *result ? 0 : -1;

remove_error:
        discard_transaction();
        *error_code = DATA_REDIS_UNAVAILABLE;
        return -1;
    }
    *error_code = DATA_INTERNAL_ERROR;
    return -1;
}

static int replace_profiles(access_profile_t *profiles, size_t count,
        const char *generation_id, const char **error_code)
{
    redisReply *reply, *old_ids;
    json_t *included;
    size_t i;
    int attempt, committed;

    included = json_object();
    if(!included) {
        *error_code = DATA_INTERNAL_ERROR;
        return -1;
    }
    for(i = 0; i < count; i++)
        json_object_set_new(included, profiles[i].device_access_id, json_true());

    for(attempt = 0; attempt < CAS_ATTEMPTS; attempt++) {
        reply = redisCommand(redis_ctx, "WATCH %s", KEY_PROFILE_REVISION);
        if(!reply_status_ok(reply)) {
            if(reply)
                freeReplyObject(reply);
            goto redis_error;
        }
        freeReplyObject(reply);
        old_ids = redisCommand(redis_ctx, "SMEMBERS %s", KEY_PROFILE_IDS);
        if(!old_ids || old_ids->type != REDIS_REPLY_ARRAY) {
            if(old_ids)
                freeReplyObject(old_ids);
            unwatch();
            goto redis_error;
        }
        if(redis_simple("MULTI") < 0) {
            freeReplyObject(old_ids);
            unwatch();
            goto redis_error;
        }
        reply = redisCommand(redis_ctx, "DEL %s", KEY_PROFILE_IDS);
        if(queue_ok(reply) < 0)
            goto replace_queue_error;
        for(i = 0; i < count; i++) {
            if(queue_profile(&profiles[i]) < 0)
                goto replace_queue_error;
            reply = redisCommand(redis_ctx, "SADD %s %s", KEY_PROFILE_IDS,
                    profiles[i].device_access_id);
            if(queue_ok(reply) < 0)
                goto replace_queue_error;
            if(!profiles[i].enabled
                    && queue_clear_registration(profiles[i].device_access_id) < 0)
                goto replace_queue_error;
        }
        for(i = 0; i < old_ids->elements; i++) {
            const char *old_id = old_ids->element[i]->str;
            char old_key[64];
            if(!old_id || json_object_get(included, old_id))
                continue;
            profile_key(old_id, old_key);
            reply = redisCommand(redis_ctx, "DEL %s", old_key);
            if(queue_ok(reply) < 0 || queue_clear_registration(old_id) < 0)
                goto replace_queue_error;
        }
        reply = redisCommand(redis_ctx, "SET %s %s", KEY_GENERATION,
                generation_id);
        if(queue_ok(reply) < 0)
            goto replace_queue_error;
        reply = redisCommand(redis_ctx, "INCR %s", KEY_PROFILE_REVISION);
        if(queue_ok(reply) < 0)
            goto replace_queue_error;
        freeReplyObject(old_ids);
        committed = exec_transaction();
        if(committed < 0)
            goto redis_error;
        if(committed == 0)
            continue;
        json_decref(included);
        return 0;

replace_queue_error:
        freeReplyObject(old_ids);
        discard_transaction();
        goto redis_error;
    }
    json_decref(included);
    *error_code = DATA_INTERNAL_ERROR;
    return -1;

redis_error:
    json_decref(included);
    *error_code = DATA_REDIS_UNAVAILABLE;
    return -1;
}

static json_t *registration_json(const char *device_id, redisReply *hash)
{
    json_t *object;
    size_t i;

    if(!hash || hash->type != REDIS_REPLY_ARRAY || hash->elements % 2 != 0)
        return NULL;
    object = json_pack("{s:s}", "device_access_id", device_id);
    if(!object)
        return NULL;
    for(i = 0; i < hash->elements; i += 2) {
        const char *field = hash->element[i]->str;
        const char *value = hash->element[i + 1]->str;
        if(!field || !value)
            goto fail;
        if(strcmp(field, "state") == 0 || strcmp(field, "reason") == 0
                || strcmp(field, "remote_address") == 0
                || strcmp(field, "expires_at") == 0
                || strcmp(field, "last_seen") == 0) {
            if(json_object_set_new(object, field, json_string(value)) < 0)
                goto fail;
        }
    }
    if(!json_object_get(object, "state")
            && json_object_set_new(object, "state", json_string("online")) < 0)
        goto fail;
    return object;

fail:
    json_decref(object);
    return NULL;
}

static void utc_now(char output[32])
{
    time_t now = time(NULL);
    struct tm value;
    gmtime_r(&now, &value);
    strftime(output, 32, "%Y-%m-%dT%H:%M:%SZ", &value);
}

static json_t *runtime_snapshot(const char **error_code)
{
    redisReply *ids, *hash;
    json_t *registrations, *entry, *result;
    char key[72], snapshot_at[32];
    long long latest;
    size_t i;

    if(redis_get_ll(KEY_EVENT_SEQUENCE, &latest) < 0)
        goto redis_error;
    ids = redisCommand(redis_ctx, "SMEMBERS %s", KEY_REGISTRATION_IDS);
    if(!ids || ids->type != REDIS_REPLY_ARRAY) {
        if(ids)
            freeReplyObject(ids);
        goto redis_error;
    }
    registrations = json_array();
    if(!registrations) {
        freeReplyObject(ids);
        *error_code = DATA_INTERNAL_ERROR;
        return NULL;
    }
    for(i = 0; i < ids->elements; i++) {
        const char *device_id = ids->element[i]->str;
        if(!device_id)
            continue;
        registration_key(device_id, key);
        hash = redisCommand(redis_ctx, "HGETALL %s", key);
        if(!hash) {
            json_decref(registrations);
            freeReplyObject(ids);
            goto redis_error;
        }
        if(hash->type == REDIS_REPLY_ARRAY && hash->elements > 0) {
            entry = registration_json(device_id, hash);
            if(!entry || json_array_append_new(registrations, entry) < 0) {
                if(entry)
                    json_decref(entry);
                freeReplyObject(hash);
                json_decref(registrations);
                freeReplyObject(ids);
                *error_code = DATA_INTERNAL_ERROR;
                return NULL;
            }
        }
        freeReplyObject(hash);
    }
    freeReplyObject(ids);
    utc_now(snapshot_at);
    result = json_pack("{s:s,s:s,s:s,s:I,s:o}",
            "access_instance_id", access_instance_id,
            "session_epoch", session_epoch,
            "snapshot_at", snapshot_at,
            "latest_sequence", (json_int_t)latest,
            "registrations", registrations);
    if(!result)
        json_decref(registrations);
    return result;

redis_error:
    *error_code = DATA_REDIS_UNAVAILABLE;
    return NULL;
}

static json_t *poll_events(long long after, int limit, const char **error_code)
{
    redisReply *reply;
    json_t *events, *event, *result;
    long long acked, latest, start;
    char start_id[32];
    size_t i, j;

    if(redis_get_ll(KEY_ACKED, &acked) < 0
            || redis_get_ll(KEY_EVENT_SEQUENCE, &latest) < 0)
        goto redis_error;
    start = after > acked ? after + 1 : acked + 1;
    snprintf(start_id, sizeof(start_id), "%lld-0", start);
    reply = redisCommand(redis_ctx, "XRANGE %s %s + COUNT %d", KEY_EVENTS,
            start_id, limit);
    if(!reply || reply->type != REDIS_REPLY_ARRAY) {
        if(reply)
            freeReplyObject(reply);
        goto redis_error;
    }
    events = json_array();
    if(!events) {
        freeReplyObject(reply);
        *error_code = DATA_INTERNAL_ERROR;
        return NULL;
    }
    for(i = 0; i < reply->elements; i++) {
        redisReply *stream_entry = reply->element[i];
        const char *encoded = NULL;
        if(!stream_entry || stream_entry->type != REDIS_REPLY_ARRAY
                || stream_entry->elements != 2
                || stream_entry->element[1]->type != REDIS_REPLY_ARRAY)
            goto invalid_stream;
        for(j = 0; j + 1 < stream_entry->element[1]->elements; j += 2) {
            redisReply *field = stream_entry->element[1]->element[j];
            redisReply *value = stream_entry->element[1]->element[j + 1];
            if(field && value && field->str && value->str
                    && strcmp(field->str, "json") == 0) {
                encoded = value->str;
                break;
            }
        }
        if(!encoded)
            goto invalid_stream;
        event = json_loads(encoded, JSON_REJECT_DUPLICATES, NULL);
        if(!event || !json_is_object(event)
                || json_array_append_new(events, event) < 0) {
            if(event)
                json_decref(event);
            goto invalid_stream;
        }
    }
    freeReplyObject(reply);
    result = json_pack("{s:s,s:s,s:I,s:o}",
            "access_instance_id", access_instance_id,
            "session_epoch", session_epoch,
            "latest_sequence", (json_int_t)latest,
            "events", events);
    if(!result)
        json_decref(events);
    return result;

invalid_stream:
    freeReplyObject(reply);
    json_decref(events);
    *error_code = DATA_INTERNAL_ERROR;
    return NULL;
redis_error:
    *error_code = DATA_REDIS_UNAVAILABLE;
    return NULL;
}

static int ack_events(long long through, const char **error_code)
{
    redisReply *reply;
    long long latest, current;
    char threshold[32];
    int attempt, committed;

    if(redis_get_ll(KEY_EVENT_SEQUENCE, &latest) < 0) {
        *error_code = DATA_REDIS_UNAVAILABLE;
        return -1;
    }
    if(through > latest) {
        *error_code = DATA_INVALID_CURSOR;
        return -1;
    }
    for(attempt = 0; attempt < CAS_ATTEMPTS; attempt++) {
        reply = redisCommand(redis_ctx, "WATCH %s", KEY_ACKED);
        if(!reply_status_ok(reply)) {
            if(reply)
                freeReplyObject(reply);
            *error_code = DATA_REDIS_UNAVAILABLE;
            return -1;
        }
        freeReplyObject(reply);
        if(redis_get_ll(KEY_ACKED, &current) < 0) {
            unwatch();
            *error_code = DATA_REDIS_UNAVAILABLE;
            return -1;
        }
        if(through <= current) {
            unwatch();
            return 0;
        }
        if(redis_simple("MULTI") < 0) {
            unwatch();
            *error_code = DATA_REDIS_UNAVAILABLE;
            return -1;
        }
        reply = redisCommand(redis_ctx, "SET %s %lld", KEY_ACKED, through);
        if(queue_ok(reply) < 0)
            goto ack_error;
        snprintf(threshold, sizeof(threshold), "%lld-0", through + 1);
        reply = redisCommand(redis_ctx, "XTRIM %s MINID %s", KEY_EVENTS,
                threshold);
        if(queue_ok(reply) < 0)
            goto ack_error;
        committed = exec_transaction();
        if(committed < 0) {
            *error_code = DATA_REDIS_UNAVAILABLE;
            return -1;
        }
        if(committed == 0)
            continue;
        return 0;

ack_error:
        discard_transaction();
        *error_code = DATA_REDIS_UNAVAILABLE;
        return -1;
    }
    *error_code = DATA_INTERNAL_ERROR;
    return -1;
}

static int dispatch_method(struct sip_msg *msg, json_t *id,
        const char *method, json_t *params)
{
    const char *error_code = DATA_INTERNAL_ERROR;
    json_t *result = NULL, *value, *profiles_value, *seen;
    access_profile_t profile, *profiles = NULL;
    const char *device_id, *generation_id;
    json_int_t integer;
    size_t i, count;
    int rc;

    if(redis_connect() < 0)
        return send_error(msg, id, -32000, "Access Redis unavailable",
                DATA_REDIS_UNAVAILABLE);

    if(strcmp(method, "access.v1.applyDeviceProfile") == 0) {
        static const char *const fields[] = {"request_id", "profile"};
        if(!object_has_only(params, fields, 2) || !required_request_id(params)
                || parse_profile(json_object_get(params, "profile"), &profile) < 0)
            return send_error(msg, id, -32602, "Invalid profile",
                    DATA_INVALID_PROFILE);
        rc = apply_profile(&profile, &result, &error_code);
    } else if(strcmp(method, "access.v1.removeDeviceProfile") == 0) {
        static const char *const fields[] = {"request_id", "device_access_id",
            "version"};
        value = json_object_get(params, "device_access_id");
        device_id = json_is_string(value) ? json_string_value(value) : NULL;
        value = json_object_get(params, "version");
        if(!object_has_only(params, fields, 3) || !required_request_id(params)
                || !valid_device_id(device_id) || !json_is_integer(value)
                || (integer = json_integer_value(value)) <= 0
                || integer > LLONG_MAX)
            return send_error(msg, id, -32602, "Invalid profile removal",
                    DATA_INVALID_PROFILE);
        rc = remove_profile(device_id, (long long)integer, &result, &error_code);
    } else if(strcmp(method, "access.v1.replaceDeviceProfiles") == 0) {
        static const char *const fields[] = {"request_id", "generation_id",
            "profiles"};
        value = json_object_get(params, "generation_id");
        generation_id = json_is_string(value) ? json_string_value(value) : NULL;
        profiles_value = json_object_get(params, "profiles");
        if(!object_has_only(params, fields, 3) || !required_request_id(params)
                || !valid_uuid(generation_id) || !json_is_array(profiles_value)
                || (count = json_array_size(profiles_value)) > MAX_PROFILES)
            return send_error(msg, id, -32602, "Invalid profile replacement",
                    DATA_INVALID_PROFILE);
        profiles = calloc(count ? count : 1, sizeof(*profiles));
        seen = json_object();
        if(!profiles || !seen) {
            free(profiles);
            if(seen)
                json_decref(seen);
            return send_error(msg, id, -32603, "Internal error",
                    DATA_INTERNAL_ERROR);
        }
        for(i = 0; i < count; i++) {
            if(parse_profile(json_array_get(profiles_value, i), &profiles[i]) < 0
                    || json_object_get(seen, profiles[i].device_access_id)) {
                free(profiles);
                json_decref(seen);
                return send_error(msg, id, -32602,
                        "Invalid or duplicate profile", DATA_INVALID_PROFILE);
            }
            json_object_set_new(seen, profiles[i].device_access_id, json_true());
        }
        json_decref(seen);
        rc = replace_profiles(profiles, count, generation_id, &error_code);
        free(profiles);
        if(rc == 0)
            result = json_pack("{s:s,s:i}", "status", "applied",
                    "profile_count", (int)count);
    } else if(strcmp(method, "access.v1.getRuntimeSnapshot") == 0) {
        static const char *const fields[] = {"request_id"};
        if(!object_has_only(params, fields, 1) || !required_request_id(params))
            return send_error(msg, id, -32602, "Invalid snapshot request",
                    DATA_INVALID_PROFILE);
        result = runtime_snapshot(&error_code);
        rc = result ? 0 : -1;
    } else if(strcmp(method, "access.v1.pollEvents") == 0) {
        static const char *const fields[] = {"request_id", "after_sequence",
            "limit"};
        json_int_t after, limit;
        value = json_object_get(params, "after_sequence");
        if(!json_is_integer(value))
            goto invalid_poll;
        after = json_integer_value(value);
        value = json_object_get(params, "limit");
        if(!json_is_integer(value))
            goto invalid_poll;
        limit = json_integer_value(value);
        if(!object_has_only(params, fields, 3) || !required_request_id(params)
                || after < 0 || after > LLONG_MAX || limit < 1 || limit > 500)
            goto invalid_poll;
        result = poll_events((long long)after, (int)limit, &error_code);
        rc = result ? 0 : -1;
        goto dispatched;
invalid_poll:
        return send_error(msg, id, -32602, "Invalid event cursor",
                DATA_INVALID_CURSOR);
    } else if(strcmp(method, "access.v1.ackEvents") == 0) {
        static const char *const fields[] = {"request_id", "through_sequence"};
        value = json_object_get(params, "through_sequence");
        if(!object_has_only(params, fields, 2) || !required_request_id(params)
                || !json_is_integer(value)
                || (integer = json_integer_value(value)) < 0
                || integer > LLONG_MAX)
            return send_error(msg, id, -32602, "Invalid event cursor",
                    DATA_INVALID_CURSOR);
        rc = ack_events((long long)integer, &error_code);
        if(rc == 0)
            result = json_pack("{s:s,s:I}", "status", "acknowledged",
                    "through_sequence", integer);
    } else {
        return send_error(msg, id, -32601, "Method not found", NULL);
    }

dispatched:
    if(rc < 0)
        return send_error(msg, id, -32000, "Access operation failed",
                error_code);
    if(!result)
        return send_error(msg, id, -32603, "Internal error",
                DATA_INTERNAL_ERROR);
    return send_result(msg, id, result);
}

static int w_rpc_dispatch(struct sip_msg *msg, char *p1, char *p2)
{
    static const char *const fields[] = {"jsonrpc", "id", "method", "params"};
    char *body;
    long content_length;
    json_error_t parse_error;
    json_t *request, *id, *params, *value;
    const char *method;
    int result;

    (void)p1;
    (void)p2;
    if(parse_headers(msg, HDR_CONTENTLENGTH_F, 0) < 0 || !msg->content_length)
        return send_error(msg, NULL, -32700, "Parse error", NULL);
    content_length = get_content_length(msg);
    body = get_body(msg);
    if(!body || content_length <= 0 || content_length > RPC_BODY_MAX
            || body < msg->buf || body + content_length > msg->buf + msg->len)
        return send_error(msg, NULL, -32700, "Parse error", NULL);
    request = json_loadb(body, (size_t)content_length,
            JSON_REJECT_DUPLICATES, &parse_error);
    if(!request)
        return send_error(msg, NULL, -32700, "Parse error", NULL);
    if(!object_has_only(request, fields, 4)) {
        json_decref(request);
        return send_error(msg, NULL, -32600, "Invalid Request", NULL);
    }
    value = json_object_get(request, "jsonrpc");
    id = json_object_get(request, "id");
    params = json_object_get(request, "params");
    value = json_object_get(request, "method");
    method = json_is_string(value) ? json_string_value(value) : NULL;
    if(!json_is_string(json_object_get(request, "jsonrpc"))
            || strcmp(json_string_value(json_object_get(request, "jsonrpc")),
                       "2.0") != 0
            || !json_is_string(id) || !valid_uuid(json_string_value(id))
            || !method || !json_is_object(params)) {
        json_decref(request);
        return send_error(msg, NULL, -32600, "Invalid Request", NULL);
    }
    result = dispatch_method(msg, id, method, params);
    json_decref(request);
    return result;
}

static int initialize_identity(void)
{
    const char *configured = getenv("NV_ACCESS_INSTANCE_ID");
    redisReply *reply, *ids;
    int changed = 0;
    size_t i;

    if(!valid_instance_id(configured) || generate_uuid(session_epoch) < 0)
        return -1;
    strcpy(access_instance_id, configured);
    if(redis_connect() < 0)
        return -1;
    reply = redisCommand(redis_ctx, "GET %s", KEY_INSTANCE);
    if(!reply)
        return -1;
    if(reply->type == REDIS_REPLY_STRING && reply->str
            && strcmp(reply->str, access_instance_id) != 0)
        changed = 1;
    else if(reply->type != REDIS_REPLY_STRING && reply->type != REDIS_REPLY_NIL) {
        freeReplyObject(reply);
        return -1;
    }
    freeReplyObject(reply);
    ids = redisCommand(redis_ctx, "SMEMBERS %s", KEY_REGISTRATION_IDS);
    if(!ids || ids->type != REDIS_REPLY_ARRAY) {
        if(ids)
            freeReplyObject(ids);
        return -1;
    }
    if(redis_simple("MULTI") < 0) {
        freeReplyObject(ids);
        return -1;
    }
    for(i = 0; i < ids->elements; i++) {
        char key[72];
        if(!ids->element[i]->str)
            continue;
        registration_key(ids->element[i]->str, key);
        reply = redisCommand(redis_ctx, "DEL %s", key);
        if(queue_ok(reply) < 0)
            goto identity_error;
    }
    reply = redisCommand(redis_ctx, "DEL %s", KEY_REGISTRATION_IDS);
    if(queue_ok(reply) < 0)
        goto identity_error;
    if(changed) {
        reply = redisCommand(redis_ctx, "DEL %s %s %s", KEY_EVENTS,
                KEY_EVENT_SEQUENCE, KEY_ACKED);
        if(queue_ok(reply) < 0)
            goto identity_error;
    }
    reply = redisCommand(redis_ctx, "SET %s %s", KEY_INSTANCE,
            access_instance_id);
    if(queue_ok(reply) < 0)
        goto identity_error;
    reply = redisCommand(redis_ctx, "SET %s %s", KEY_EPOCH, session_epoch);
    if(queue_ok(reply) < 0)
        goto identity_error;
    freeReplyObject(ids);
    return exec_transaction() == 1 ? 0 : -1;

identity_error:
    freeReplyObject(ids);
    discard_transaction();
    return -1;
}

static int w_authorize_register(struct sip_msg *msg, char *p1, char *p2)
{
    char device_id[21];
    access_profile_t profile;
    int result;

    (void)p1;
    (void)p2;
    result = load_request_profile(msg, device_id, &profile);
    return result == 1 ? 1 : (result == -2 ? -2 : -1);
}

static int w_record_registration(struct sip_msg *msg, char *p1, char *p2)
{
    char device_id[21];
    access_profile_t profile;
    unsigned int expires;
    int result;

    (void)p1;
    (void)p2;
    result = load_request_profile(msg, device_id, &profile);
    if(result != 1)
        return result == -2 ? -2 : -1;
    if(parse_register_expires(msg, &expires) < 0)
        return -1;
    if(expires == 0)
        return runtime_unregister(msg, device_id, "unregister");
    return runtime_register(msg, device_id, expires) > 0 ? 1 : -2;
}

static int w_handle_keepalive(struct sip_msg *msg, char *p1, char *p2)
{
    char device_id[21], key[72], last_seen[32];
    access_profile_t profile;
    redisReply *reply;
    int result;
    long long deadline;

    (void)p1;
    (void)p2;
    result = load_request_profile(msg, device_id, &profile);
    if(result != 1)
        return result == -2 ? -3 : -1;
    if(parse_keepalive_body(msg, device_id) < 0)
        return -2;
    registration_key(device_id, key);
    reply = redisCommand(redis_ctx, "HGET %s session_epoch", key);
    if(!reply)
        return -3;
    if(reply->type != REDIS_REPLY_STRING || !reply->str
            || strcmp(reply->str, session_epoch) != 0) {
        freeReplyObject(reply);
        return -1;
    }
    freeReplyObject(reply);
    reply = redisCommand(redis_ctx, "HGET %s state", key);
    if(!reply)
        return -3;
    if(reply->type != REDIS_REPLY_STRING || !reply->str
            || strcmp(reply->str, "online") != 0) {
        freeReplyObject(reply);
        return -1;
    }
    freeReplyObject(reply);
    utc_now(last_seen);
    deadline = unix_now() + keepalive_timeout();
    reply = redisCommand(redis_ctx,
            "HSET %s last_seen %s keepalive_deadline %lld", key, last_seen,
            deadline);
    if(!reply_status_ok(reply)) {
        if(reply) freeReplyObject(reply);
        return -3;
    }
    freeReplyObject(reply);
    return 1;
}

static int w_ready(struct sip_msg *msg, char *p1, char *p2)
{
    (void)msg;
    (void)p1;
    (void)p2;
    return redis_ping();
}

static int extract_request_device_id(struct sip_msg *msg, char output[21])
{
    struct to_body *from;
    struct sip_uri uri;

    if(parse_from_header(msg) < 0 || !msg->from)
        return -1;
    from = get_from(msg);
    if(!from || parse_uri(from->uri.s, from->uri.len, &uri) < 0
            || uri.user.len != 20)
        return -1;
    memcpy(output, uri.user.s, 20);
    output[20] = '\0';
    return valid_device_id(output) ? 0 : -1;
}

static int set_request_avp(const char *name, const char *value)
{
    avp_name_t avp_name;
    avp_value_t avp_value;
    str name_str = {(char *)name, (int)strlen(name)};
    str value_str = {(char *)value, (int)strlen(value)};

    avp_name.s = name_str;
    avp_value.s = value_str;
    delete_avp(AVP_NAME_STR, avp_name);
    return add_avp(AVP_NAME_STR | AVP_VAL_STR, avp_name, avp_value);
}

/* Return 0 for an unknown/disabled device, -2 for an Access backend error. */
static int load_request_profile(struct sip_msg *msg, char device_id[21],
        access_profile_t *profile)
{
    int loaded;

    if(extract_request_device_id(msg, device_id) < 0)
        return 0;
    if(redis_connect() < 0)
        return -2;
    loaded = load_profile(device_id, profile);
    if(loaded < 0)
        return -2;
    if(!profile->exists || profile->tombstone || !profile->enabled
            || strcmp(profile->device_access_id, device_id) != 0
            || strcmp(profile->sip_username, device_id) != 0)
        return 0;
    if(set_request_avp("gb_realm", profile->sip_realm) < 0
            || set_request_avp("gb_ha1", profile->digest_ha1) < 0)
        return -2;
    return 1;
}

static int parse_register_expires(struct sip_msg *msg, unsigned int *expires)
{
    exp_body_t *body;

    *expires = 3600;
    if(parse_headers(msg, HDR_EXPIRES_F, 0) < 0)
        return -1;
    if(!msg->expires)
        return 0;
    if(parse_expires(msg->expires) < 0 || !msg->expires->parsed)
        return -1;
    body = (exp_body_t *)msg->expires->parsed;
    if(!body->valid)
        return -1;
    *expires = body->val;
    return 0;
}

static long long unix_now(void)
{
    return (long long)time(NULL);
}

static int emit_registration_event(const char *device_id, const char *state,
        const char *reason, const char *remote_address, const char *expires_at,
        const char *last_seen)
{
    json_t *payload, *event;
    char *encoded = NULL;
    char event_id[128];
    redisReply *reply;
    long long current, next;
    int attempt, committed;

    payload = json_pack("{s:s,s:s,s:s}", "state", state, "reason", reason,
            "remote_address", remote_address ? remote_address : "");
    if(!payload
            || (expires_at && *expires_at
                && json_object_set_new(payload, "expires_at",
                    json_string(expires_at)) < 0)
            || json_object_set_new(payload, "last_seen", json_string(last_seen)) < 0) {
        if(payload)
            json_decref(payload);
        return -1;
    }
    for(attempt = 0; attempt < CAS_ATTEMPTS; attempt++) {
        reply = redisCommand(redis_ctx, "WATCH %s", KEY_EVENT_SEQUENCE);
        if(!reply_status_ok(reply)) {
            if(reply)
                freeReplyObject(reply);
            json_decref(payload);
            return -1;
        }
        freeReplyObject(reply);
        if(redis_get_ll(KEY_EVENT_SEQUENCE, &current) < 0
                || current == LLONG_MAX) {
            unwatch();
            json_decref(payload);
            return -1;
        }
        next = current + 1;
        snprintf(event_id, sizeof(event_id), "%s:%lld", access_instance_id, next);
        event = json_pack("{s:s,s:I,s:s,s:s,s:s,s:s,s:O}", "event_id",
                event_id, (json_int_t)next, "access_instance_id",
                access_instance_id, "session_epoch", session_epoch,
                "type", "registration_changed", "occurred_at", last_seen,
                "device_access_id", device_id, "payload", payload);
        if(!event) {
            unwatch();
            json_decref(payload);
            return -1;
        }
        encoded = json_dumps(event, JSON_COMPACT | JSON_ENSURE_ASCII);
        json_decref(event);
        if(!encoded) {
            unwatch();
            return -1;
        }
        if(redis_simple("MULTI") < 0) {
            unwatch();
            free(encoded);
            return -1;
        }
        reply = redisCommand(redis_ctx, "SET %s %lld", KEY_EVENT_SEQUENCE,
                next);
        if(queue_ok(reply) < 0)
            goto event_error;
        reply = redisCommand(redis_ctx, "XADD %s %lld-0 json %b", KEY_EVENTS,
                next, encoded, strlen(encoded));
        if(queue_ok(reply) < 0)
            goto event_error;
        free(encoded);
        encoded = NULL;
        committed = exec_transaction();
        if(committed < 0) {
            json_decref(payload);
            return -1;
        }
        if(committed == 0)
            continue;
        json_decref(payload);
        return 0;

event_error:
        discard_transaction();
        free(encoded);
        json_decref(payload);
        return -1;
    }
    json_decref(payload);
    return -1;
}

static int runtime_register(struct sip_msg *msg, const char *device_id,
        unsigned int expires)
{
    char key[72], remote[128], expires_at[32], last_seen[32];
    const char *address;
    redisReply *reply;
    long long expiry;
    int was_online = 0;
    char previous_epoch[37] = {0};

    registration_key(device_id, key);
    reply = redisCommand(redis_ctx, "HGET %s state", key);
    if(!reply) return -1;
    if(reply->type == REDIS_REPLY_STRING && reply->str
            && strcmp(reply->str, "online") == 0) {
        was_online = 1;
    }
    freeReplyObject(reply);
    reply = redisCommand(redis_ctx, "HGET %s session_epoch", key);
    if(reply && reply->type == REDIS_REPLY_STRING && reply->str)
        snprintf(previous_epoch, sizeof(previous_epoch), "%s", reply->str);
    if(reply) freeReplyObject(reply);
    address = ip_addr2a(&msg->rcv.src_ip);
    snprintf(remote, sizeof(remote), "%s:%u", address ? address : "unknown",
            (unsigned int)msg->rcv.src_port);
    expiry = unix_now() + expires;
    utc_now(last_seen);
    {
        time_t expiry_time = (time_t)expiry;
        struct tm value;
        gmtime_r(&expiry_time, &value);
        strftime(expires_at, sizeof(expires_at), "%Y-%m-%dT%H:%M:%SZ", &value);
    }
    if(redis_simple("MULTI") < 0)
        return -1;
    reply = redisCommand(redis_ctx,
            "HSET %s device_access_id %s state online reason register "
            "remote_address %s expires_at %s expires_unix %lld last_seen %s "
            "keepalive_deadline %lld session_epoch %s",
            key, device_id, remote, expires_at, expiry, last_seen,
            unix_now() + keepalive_timeout(), session_epoch);
    if(queue_ok(reply) < 0)
        goto runtime_error;
    reply = redisCommand(redis_ctx, "SADD %s %s", KEY_REGISTRATION_IDS,
            device_id);
    if(queue_ok(reply) < 0)
        goto runtime_error;
    if(expires == 0) {
        /* REGISTER Expires: 0 is handled by runtime_unregister(). */
        discard_transaction();
        return -1;
    }
    if(exec_transaction() != 1)
        return -1;
    if(!was_online || strcmp(previous_epoch, session_epoch) != 0) {
        if(emit_registration_event(device_id, "online", "register", remote,
                    expires_at, last_seen) < 0)
            return -1;
    }
    return 1;

runtime_error:
    discard_transaction();
    return -1;
}

static int runtime_unregister(struct sip_msg *msg, const char *device_id,
        const char *reason)
{
    char key[72], remote[128], last_seen[32];
    const char *address = ip_addr2a(&msg->rcv.src_ip);
    redisReply *reply;
    int was_online = 0;

    registration_key(device_id, key);
    reply = redisCommand(redis_ctx, "HGET %s state", key);
    if(!reply) return -1;
    was_online = reply->type == REDIS_REPLY_STRING && reply->str
            && strcmp(reply->str, "online") == 0;
    freeReplyObject(reply);
    snprintf(remote, sizeof(remote), "%s:%u", address ? address : "unknown",
            (unsigned int)msg->rcv.src_port);
    utc_now(last_seen);
    if(redis_simple("MULTI") < 0)
        return -1;
    reply = redisCommand(redis_ctx, "DEL %s", key);
    if(queue_ok(reply) < 0)
        goto unregister_error;
    reply = redisCommand(redis_ctx, "SREM %s %s", KEY_REGISTRATION_IDS,
            device_id);
    if(queue_ok(reply) < 0)
        goto unregister_error;
    if(exec_transaction() != 1)
        return -1;
    if(was_online && emit_registration_event(device_id, "offline", reason,
                remote, "", last_seen) < 0)
        return -1;
    return 1;

unregister_error:
    discard_transaction();
    return -1;
}

static int xml_contains_doctype(const char *body, size_t length)
{
    size_t i, j;
    static const char marker[] = "<!doctype";

    for(i = 0; i + sizeof(marker) - 1 <= length; i++) {
        for(j = 0; j < sizeof(marker) - 1; j++) {
            if(tolower((unsigned char)body[i + j]) != marker[j])
                break;
        }
        if(j == sizeof(marker) - 1)
            return 1;
    }
    return 0;
}

static int parse_keepalive_body(struct sip_msg *msg, const char *device_id)
{
    char *body;
    long content_length;
    xmlDocPtr doc = NULL;
    xmlNodePtr root, child;
    xmlChar *cmd = NULL, *reported_id = NULL;
    int valid = 0;

    if(parse_headers(msg, HDR_CONTENTLENGTH_F | HDR_CONTENTTYPE_F, 0) < 0
            || !msg->content_length || !msg->content_type)
        return -2;
    if(msg->content_type->body.len < (int)(sizeof("application/MANSCDP+xml") - 1)
            || strncasecmp(msg->content_type->body.s,
                    "application/MANSCDP+xml", sizeof("application/MANSCDP+xml") - 1) != 0
            || (msg->content_type->body.len > (int)(sizeof("application/MANSCDP+xml") - 1)
                && msg->content_type->body.s[sizeof("application/MANSCDP+xml") - 1] != ';'
                && !isspace((unsigned char)msg->content_type->body.s[sizeof("application/MANSCDP+xml") - 1])))
        return -2;
    content_length = get_content_length(msg);
    body = get_body(msg);
    if(!body || content_length <= 0 || content_length > 65536
            || body < msg->buf || body + content_length > msg->buf + msg->len
            || xml_contains_doctype(body, (size_t)content_length))
        return -2;
    doc = xmlReadMemory(body, (int)content_length, "keepalive.xml", NULL,
            XML_PARSE_NONET | XML_PARSE_NOERROR | XML_PARSE_NOWARNING);
    if(!doc)
        return -2;
    root = xmlDocGetRootElement(doc);
    if(!root || xmlStrcasecmp(root->name, (const xmlChar *)"Notify") != 0)
        goto done;
    for(child = root->children; child; child = child->next) {
        if(child->type != XML_ELEMENT_NODE)
            continue;
        if(xmlStrcasecmp(child->name, (const xmlChar *)"CmdType") == 0)
            cmd = xmlNodeGetContent(child);
        else if(xmlStrcasecmp(child->name, (const xmlChar *)"DeviceID") == 0)
            reported_id = xmlNodeGetContent(child);
    }
    valid = cmd && reported_id && xmlStrcasecmp(cmd,
            (const xmlChar *)"Keepalive") == 0
            && strcmp((const char *)reported_id, device_id) == 0;
done:
    if(cmd) xmlFree(cmd);
    if(reported_id) xmlFree(reported_id);
    xmlFreeDoc(doc);
    return valid ? 0 : -2;
}

static int keepalive_timeout(void)
{
    const char *value = getenv("NV_KEEPALIVE_TIMEOUT");
    long parsed;
    char *end;

    if(!value || !*value)
        return 180;
    errno = 0;
    parsed = strtol(value, &end, 10);
    if(errno || *end || parsed < 1 || parsed > 86400)
        return 180;
    return (int)parsed;
}

static void access_timer(unsigned int ticks, void *param)
{
    redisReply *ids, *reply;
    size_t i;
    long long now = unix_now(), expiry, keepalive;
    char key[72], remote[128], expires_at[32], last_seen[32], *end;
    const char *reason;

    (void)ticks;
    (void)param;
    if(redis_connect() < 0)
        return;
    ids = redisCommand(redis_ctx, "SMEMBERS %s", KEY_REGISTRATION_IDS);
    if(!ids || ids->type != REDIS_REPLY_ARRAY) {
        if(ids) freeReplyObject(ids);
        return;
    }
    for(i = 0; i < ids->elements; i++) {
        const char *device_id = ids->element[i]->str;
        int expired = 0;
        if(!device_id) continue;
        registration_key(device_id, key);
        reply = redisCommand(redis_ctx, "WATCH %s", key);
        if(!reply_status_ok(reply)) {
            if(reply) freeReplyObject(reply);
            continue;
        }
        freeReplyObject(reply);
        reply = redisCommand(redis_ctx, "HGET %s state", key);
        if(!reply || reply->type != REDIS_REPLY_STRING || !reply->str
                || strcmp(reply->str, "online") != 0) {
            if(reply) freeReplyObject(reply);
            unwatch();
            continue;
        }
        freeReplyObject(reply);
        expiry = 0;
        keepalive = 0;
        reply = redisCommand(redis_ctx, "HGET %s expires_unix", key);
        if(reply && reply->type == REDIS_REPLY_STRING && reply->str) {
            errno = 0;
            expiry = strtoll(reply->str, &end, 10);
            if(errno || *end) expiry = 0;
        }
        if(reply) freeReplyObject(reply);
        reply = redisCommand(redis_ctx, "HGET %s keepalive_deadline", key);
        if(reply && reply->type == REDIS_REPLY_STRING && reply->str) {
            errno = 0;
            keepalive = strtoll(reply->str, &end, 10);
            if(errno || *end) keepalive = 0;
        }
        if(reply) freeReplyObject(reply);
        if(expiry > 0 && now >= expiry) {
            expired = 1;
            reason = "registration_expired";
        } else if(keepalive > 0 && now >= keepalive) {
            expired = 1;
            reason = "keepalive_timeout";
        } else {
            reason = "";
        }
        if(!expired) {
            unwatch();
            continue;
        }
        reply = redisCommand(redis_ctx, "HGET %s remote_address", key);
        snprintf(remote, sizeof(remote), "%s",
                reply && reply->type == REDIS_REPLY_STRING && reply->str
                        ? reply->str : "");
        if(reply) freeReplyObject(reply);
        reply = redisCommand(redis_ctx, "HGET %s expires_at", key);
        snprintf(expires_at, sizeof(expires_at), "%s",
                reply && reply->type == REDIS_REPLY_STRING && reply->str
                        ? reply->str : "");
        if(reply) freeReplyObject(reply);
        utc_now(last_seen);
        if(redis_simple("MULTI") < 0) {
            unwatch();
            continue;
        }
        reply = redisCommand(redis_ctx, "DEL %s", key);
        if(queue_ok(reply) < 0) {
            discard_transaction();
            continue;
        }
        reply = redisCommand(redis_ctx, "SREM %s %s", KEY_REGISTRATION_IDS,
                device_id);
        if(queue_ok(reply) < 0) {
            discard_transaction();
            continue;
        }
        if(exec_transaction() == 1)
            (void)emit_registration_event(device_id, "offline", reason, remote,
                    expires_at, last_seen);
    }
    freeReplyObject(ids);
}


static cmd_export_t cmds[] = {
    {"gb28181_ready", (cmd_function)w_ready, 0, 0, 0, ANY_ROUTE},
    {"gb28181_rpc_dispatch", (cmd_function)w_rpc_dispatch, 0, 0, 0, ANY_ROUTE},
    {"gb28181_authorize_register", (cmd_function)w_authorize_register, 0, 0, 0, REQUEST_ROUTE},
    {"gb28181_record_registration", (cmd_function)w_record_registration, 0, 0, 0, REQUEST_ROUTE},
    {"gb28181_handle_keepalive", (cmd_function)w_handle_keepalive, 0, 0, 0, REQUEST_ROUTE},
    {0, 0, 0, 0, 0, 0}
};

struct module_exports exports = {
    "gb28181",
    DEFAULT_DLFLAGS,
    cmds,
    0,
    0,
    0,
    0,
    mod_init,
    child_init,
    mod_destroy
};

static int mod_init(void)
{
    if(xhttp_load_api(&xhttp_api) < 0) {
        LM_ERR("failed to bind xhttp API\n");
        return -1;
    }
    if(initialize_identity() < 0) {
        LM_ERR("failed to initialize Access identity in Redis\n");
        return -1;
    }
    if(register_timer(access_timer, 0, 5) < 0) {
        LM_ERR("failed to register Access runtime timer\n");
        redis_close();
        return -1;
    }
    redis_close();
    return 0;
}

static int child_init(int rank)
{
    (void)rank;
    redis_ctx = NULL;
    return 0;
}

static void mod_destroy(void)
{
    redis_close();
}
