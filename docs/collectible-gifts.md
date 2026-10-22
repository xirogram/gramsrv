# Collectible Star gifts: listing flags, value info, withdrawal

What this service implements for collectible (unique) Star gifts on top of the
Layer 232 TL schema: the saved-gift listing flags,
`payments.getUniqueStarGiftValueInfo`, `payments.getStarGiftWithdrawalUrl`
(with the 2FA gate it shares with the self-hosted claim page) and
`payments.deleteStarGift`.

## Saved gift listing (`getSavedStarGifts` family)

Filter fields live in `domain.SavedStarGiftFilter`
(`internal/domain/star_gift.go`), both stores implement them
(`internal/store/postgres/star_gift.go`, `internal/store/memory/star_gift.go`).

| Flag | Meaning |
| --- | --- |
| `sort_by_value` | Order by price descending instead of the profile order. |
| `peer_color_available` | Only gifts that are unique (`unique_gift_id IS NOT NULL`) and whose active catalog revision has `peer_color_available`. |
| `exclude_hosted` | Drop rows that are only visible because this peer hosts the on-chain gift. |
| `collection_id` | Restrict to one collection; also disables the profile order. |

### Price and the `sort_by_value` cursor

The price used for sorting and paging is `priceExpr`: the XTR listing price of
the unique gift, else its recorded `value_amount` when `value_currency='XTR'`,
else the active catalog revision price in Stars. TON listings are not
converted: they fall back to the catalog price rather than inventing a rate.

The cursor for `sort_by_value` is the opaque string `v2:<price>:<id>`
(`EncodeSavedStarGiftPriceCursor` / `DecodeSavedStarGiftPriceCursor`), and the
next page is `price < cursor OR (price = cursor AND id < cursor_id)` with
`ORDER BY price DESC, id DESC`. The older list cursor (`v1:<pinned>:<id>`)
keeps working for the other orderings.

### Ordering

- `collection_id == 0 && !sort_by_value`: profile order — pinned gifts first in
  `pinned_order`, then `id DESC`.
- `sort_by_value`: price descending, ties broken by `id DESC`.
- anything else: `id DESC`.

### Hosted rows

PostgreSQL serves a gift to its owner while `lifecycle_status='active'`, and to
the host peer while the gift is exported on-chain (`lifecycle_status='exported'`,
non-empty `owner_address`, non-burned, `host_peer_*` equal to the requested
peer). `exclude_hosted` removes the second case.

The memory store only ever produces `active` rows, so `exclude_hosted` is an
explicit no-op there: no hosted row can exist to exclude. It is not a filter
for "gifts that were upgraded to unique" — `exclude_unique` is.

## `payments.getUniqueStarGiftValueInfo`

Takes a slug, normalizes it (`TrimSpace` + lowercase, at most
`domain.MaxStarGiftSlugBytes`), and returns currency/value, initial sale, and
optionally last sale, floor, average and `listed_count` when the self-hosted
ledger recorded them.

- Unknown, empty or over-long slug → `STARGIFT_SLUG_INVALID`.
- Bot callers → `BOT_METHOD_INVALID`.
- `last_sale_on_fragment` and `fragment_listed_*` are never set: this ledger has
  no Fragment feed, so the correct value is "unknown" (left absent).

## `payments.getStarGiftWithdrawalUrl`

Preconditions are checked in this order, because each later one would otherwise
reveal state to a caller who has not proven who they are:

1. Known method, non-bot caller (`BOT_METHOD_INVALID`).
2. Layer 232 replaced the SRP challenge with a temporary password: the request
   carries `tmp_password:bytes` instead of `inputCheckPasswordSRP`. Accounts
   without 2FA have no proof to check, so they get `PASSWORD_MISSING` before
   anything else. The proof itself is a grant issued by
   `account.getTmpPassword` for this session; a missing, malformed, expired or
   foreign grant fails closed (`TMP_PASSWORD_INVALID`) and is never treated as
   an SRP answer.
   Historical profiles (Layer 225–231) still send the SRP challenge; their
   constructor is promoted to a separate legacy method and checked with
   `account.CheckPassword` over `inputCheckPasswordSRP` only — an empty or
   mismatched check is `PASSWORD_HASH_INVALID`, a stale `srp_id` is
   `SRP_ID_INVALID`.
3. Freshness, only when the account has a password: `PASSWORD_TOO_FRESH_<wait>`
   for a recently changed password and `SESSION_TOO_FRESH_<wait>` for a session
   that is younger than the same window (`revenueWithdrawalFreshWait`).
4. The auth key exists, belongs to the caller, and is not `password_pending`
   (`AUTH_KEY_UNREGISTERED`).
5. The gift ref resolves and the caller is its owner (`STARGIFT_INVALID`,
   `STARGIFT_OWNER_INVALID`). Hosts of on-chain gifts are not owners and can
   never withdraw.
6. `stargifts.Service.Withdraw`: the gift must be live and its
   `can_export_at` must have passed; otherwise `ErrStarGiftExportCooldown`
   → `STARGIFT_WITHDRAWAL_UNAVAILABLE`, and any other non-exportable state
   (`STARGIFT_INVALID`).

`can_export_at` is compared against the **server** clock
(`Service.Withdraw` takes `req.Date` from the router), so a client cannot
advance time locally.

### Double withdrawal

`StarGiftLifecycleStore.RecordStarGiftWithdrawal` locks the saved gift row
(`FOR UPDATE`), then locks the `star_gift_withdrawal_requests` row for that
unique gift. A pending request that has not expired, or a completed one, is
returned as-is instead of minting a second bearer URL; an expired one is
reused/updated. Ten concurrent calls therefore collapse onto a single row and a
single URL — covered by `TestStarGiftWithdrawalConcurrentRecordCollapsesPostgres`.

## `payments.deleteStarGift`

Layer 232 added `payments.deleteStarGift#3cdaf8aa stargift:InputSavedStarGift =
Bool`. The owner removes a received gift from the profile.

- Only the caller may delete their own gift. A channel gift is a channel asset
  and is refused (`STARGIFT_OWNER_INVALID`); bots are refused
  (`BOT_METHOD_INVALID`).
- Only a **plain** received gift can be deleted. A gift that was upgraded into
  a collectible (NFT) is refused (`STARGIFT_INVALID`): destroying a
  collectible is a different operation with its own financial rules, so this
  method never burns one.
- The gift row is **not** deleted: upgrade commands, offers and transfers
  reference it, so the delete is a lifecycle transition
  (`lifecycle_status='deleted'`, new state in migration
  `20261023000001_star_gift_deleted_state`). Read paths only project
  `lifecycle_status='active'`, so the gift leaves the profile, the account
  rating signal and the saved-gift lists immediately.
- Refused states (`STARGIFT_INVALID`): converted, exported, burned, already
  deleted, upgraded into a collectible.
- The gift is dropped from every collection it was part of, and its profile
  projection is invalidated.

### Layer 232 methods that are stubs

The rest of the Layer 232 surface is decoded, so a client gets an explicit
refusal instead of an unknown-method error, but the business behind it is not
implemented yet: `messages.setShareTheme`,
`messages.getShareThemeBackdrops`, `payments.withdrawStarGift`,
`payments.depositStarGift`, `payments.displayStarGift`,
`contacts.exportNoPaidMessagesToken`, `contacts.importNoPaidMessagesToken` and
`wallet.setCardStickers` all answer `NOT_IMPLEMENTED`. They live in
`internal/rpc/layer232_stubs.go`; each stub owns one semantic registration, so
implementing one replaces exactly one handler. Note that
`payments.withdrawStarGift` failing closed also means the wallet withdraw path
stays where it was: `payments.getStarGiftWithdrawalUrl` remains the only
export route. `account.getTmpPassword` is no longer a stub: it issues the
temporary-password grants the withdrawal path checks.

## Claim page mirror (FlashFragment)

The claim page in the fragment UI posts the same proof to the server instead of
MTProto. Routes are registered in `internal/web/server.go`, handled by
`internal/giftclaim`:

| Route | Purpose |
| --- | --- |
| `POST /claim/api/wallet/password` (and `/claimtest/...`) | Rotate and return the SRP challenge. |
| `POST /claim/api/wallet/withdraw` | Confirm with `{srp_id, a, m1}` and get the withdrawal URL. |

Both require the `X-Telegram-Init-Data` header and return `401` without it.

```jsonc
// response of /wallet/password
{"has_password": true, "srp_id": "123…", "srp_b": "…", "g": 5, "p": "…", "salt1": "…", "salt2": "…"}
// request of /wallet/withdraw
{"gift": "<slug or msg ref>", "password": {"srp_id": "123…", "a": "…", "m1": "…"}}
```

`srp_id` is a random `int64` and is serialised as a **string** in both
directions (`json:",string"`); JavaScript must not round-trip it through
`Number`. `g`/`p`/salts are hex, `a`/`m1` are hex too.

Without `has_password`, the client sends no `password` object and the server
accepts `inputCheckPasswordEmpty`, exactly like MTProto. With `has_password` the
Mini App asks for the Telegram password, derives

```
x  = SHA256(salt2 || PBKDF2-HMAC-SHA512(key=SHA256(salt2||SHA256(salt1||pw||salt1)||salt2), salt=salt1, 100000, 64) || salt2)
v  = g^x mod p
u  = SHA256(pad256(A) || pad256(B))
S  = ((B - k*g^x) mod p)^(a + u*x) mod p, k = SHA256(pad256(p) || pad256(g))
M1 = SHA256( SHA256(pad256(p)) XOR SHA256(pad256(g)) || SHA256(salt1) || SHA256(salt2) || pad256(A) || pad256(B) || SHA256(pad256(S)) )
```

and posts `{srp_id, a, m1}`. The server derives `x` again from the stored
password, so the password itself never leaves the browser. The reference
implementation is `internal/app/account/srp.go`; the browser copy lives in
`fragment.com/js/openfragment-stub.js` (`srpSign`).

Errors map to the same claims codes: a bad password answer gives
`ErrPasswordInvalid` (HTTP 400), an export that is on cooldown gives HTTP 409.

nginx exposes only the exact claim paths under
`location = /fragment/api/gift-claim/wallet/...` → `:2401/claim/...` with the
init-data header injected.

## Not implemented

- `.ton` username minting and the hosted Fragment UI (withdraw only).
- `fragment_listed_*` / `last_sale_on_fragment` fields.
- Any TON-to-Stars price conversion for `sort_by_value`.

## Tests

- `internal/rpc/payments_star_gift_withdrawal_rpc_test.go` — ownership, unknown
  slug, password freshness, session freshness, bot rejection, cooldown mapping,
  value-info slug error.
- `internal/giftclaim/withdraw_password_test.go` — full challenge → SRP →
  withdraw round trip over HTTP against the real `account.Service`.
- `internal/store/memory/star_gift_value_filter_test.go` — value ordering, price
  cursor paging, `peer_color_available`, `exclude_hosted` no-op.
- `internal/domain/star_gift_price_cursor_test.go` — cursor round trip.
- `internal/store/postgres/star_gift_saved_listing_flags_integration_test.go` —
  the same flags against PostgreSQL, including hosted rows.
- `internal/store/postgres/star_gift_withdrawal_race_integration_test.go` —
  ten concurrent withdrawals collapse into one request row.

The PostgreSQL ones run only when `TELESRV_TEST_POSTGRES_DSN` names a database
whose name contains `test`.
