# dasdis

An in-memory Redis-compatible server speaking RESP2, built on top of the
[dasgo](https://github.com/rexrecio/dasgo) data structures. Lists are backed
by `dasgo/linkedlist` and sorted sets by `dasgo/avl`. There is no
persistence — all data is lost when the process exits.

## Run

```bash
go run . -addr 127.0.0.1:6380
```

Then connect with any Redis client, e.g.:

```bash
redis-cli -h 127.0.0.1 -p 6380
```

## Supported

**Connection**

- `PING [msg]`, `ECHO msg`, `QUIT`
- `SELECT`, `CLIENT`, `HELLO` (handshake only — accepted, no behavior change;
  `HELLO` with anything other than protocol 2 returns `NOPROTO`)
- `COMMAND` (returns an empty array)

**Generic keys**

- `DEL key [key ...]`, `EXISTS key [key ...]`

**Strings**

- `GET`, `SET key value` (extra options like `EX`/`PX`/`NX`/`XX` are accepted
  and silently ignored — TTL is not honored)
- `INCR`, `DECR`

**Lists** (backed by `dasgo/linkedlist`)

- `LPUSH`, `RPUSH`, `LPOP`, `RPOP`, `LLEN`, `LRANGE`

**Sorted sets** (backed by `dasgo/avl`)

- `ZADD key score member [score member ...]` (no `NX`/`XX`/`GT`/`LT`/`CH`/`INCR`
  flags)
- `ZRANGE key start stop [WITHSCORES]` (index-based only)
- `ZSCORE`, `ZREM`

**Protocol**

- RESP2 only
- Inline command form (telnet-style) accepted

## Not supported

**Persistence / admin**

- No RDB, no AOF, no replication, no clustering
- No `SAVE`, `BGSAVE`, `FLUSHALL`, `FLUSHDB`, `DBSIZE`, `KEYS`, `SCAN`,
  `INFO`, `CONFIG`, `DEBUG`, `SHUTDOWN`
- Only one logical DB; `SELECT n` is a no-op
- No AUTH / ACL — anyone who can connect has full access

**Expiration**

- No `EXPIRE`, `TTL`, `PERSIST`, `EXPIREAT`. The `EX`/`PX` options on `SET`
  are parsed but ignored.

**Strings**

- No `MGET`, `MSET`, `GETSET`, `SETNX`, `SETEX`, `INCRBY`, `DECRBY`,
  `INCRBYFLOAT`, `APPEND`, `STRLEN`, `GETRANGE`, `SETRANGE`, bit operations

**Lists**

- No `LINDEX`, `LSET`, `LINSERT`, `LREM`, `LTRIM`, `RPOPLPUSH`/`LMOVE`,
  blocking variants (`BLPOP`, `BRPOP`)
- `LPOP key count` / `RPOP key count` not supported — only the single-element
  form

**Sorted sets**

- No `ZRANGEBYSCORE`, `ZRANGEBYLEX`, `ZREVRANGE`, `ZRANK`/`ZREVRANK`,
  `ZCARD`, `ZCOUNT`, `ZINCRBY`, `ZPOPMIN`/`ZPOPMAX`,
  `ZUNIONSTORE`/`ZINTERSTORE`
- No `ZADD` modifier flags (NX/XX/GT/LT/CH/INCR)

**Whole data types missing**

- Hashes (`HSET`/`HGET`/...)
- Sets (`SADD`/`SMEMBERS`/...)
- Streams, HyperLogLog, geo, bitmaps
- Pub/Sub (`SUBSCRIBE`/`PUBLISH`)
- Transactions (`MULTI`/`EXEC`/`WATCH`)
- Scripting (`EVAL`/`FUNCTION`)
- Client tracking / RESP3 push

**Protocol**

- RESP3 not implemented (`HELLO 3` returns `NOPROTO`)

## Performance caveats

- `RPOP` is O(n) — the underlying singly-linked list has no O(1) tail removal
- One global mutex on the store — no per-key concurrency

## Test

```bash
go test ./...
```

The tests in `server/server_test.go` exercise every supported command over a
real TCP socket; no external Redis is required.

### Docker image test

`docker_test.go` builds the image from the local Dockerfile, starts a
container, and runs the same command set against the live server via
testcontainers-go. Docker must be running.

```bash
go test -v -run TestDockerImage -timeout 5m .
```
