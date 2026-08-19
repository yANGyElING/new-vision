# gb28181 Kamailio module

This module owns the Access control plane and GB28181 protocol semantics used
by `node-app` and `kamailio.cfg`.

Kamailio 6.1.3 `jsonrpcs` cannot scan the nested profile objects and arrays in
the versioned Access contract. `gb28181_rpc_dispatch()` therefore parses the
JSON-RPC 2.0 envelope with libjansson and sends the HTTP response through the
public xhttp API. It implements profile apply/remove/full replacement,
runtime snapshots, non-blocking event polling, monotonic event ACK, Redis
profile version CAS, and runtime event production.

The module uses Kamailio's standard `auth`, `registrar`, `usrloc`, `tm`, and
`sl` modules for SIP behavior. It validates the From URI against the active
profile, supplies the stored MD5 HA1 to `pv_auth_check()`, records successful
REGISTER bindings, handles Expires 0, validates bounded GB28181 KeepAlive XML
without external entities, and expires stale runtime registrations from a
module timer. Redis connections are process-local and use the ACL identity
supplied through the container environment.
