# splitgo

A minimal app for sharing expenses with friends: groups, expenses, balances and settling up.
Inspired by [split-pro](https://github.com/oss-apps/split-pro) and re-implemented as a single Go binary
with SQLite, [htmx](https://htmx.org) and [gomponents](https://www.gomponents.com).

## Features

- Passwordless sign-in with email magic links
- Groups, with invite links or adding people by email
- Expenses in any currency. The simple view offers quick options:
  - you paid, split equally
  - you are owed the full amount
  - they paid, split equally
  - they are owed the full amount

  **More options** splits equally, by percentage, by shares, by exact amounts or by adjustments.
- Per-person and per-group balances, kept separately for each currency
- Settle up, including partial payments
- Optional debt simplification per group
- An activity feed

## Running

```sh
DATABASE_DIR=./data BASE_URL=http://localhost:8080 go run ./cmd/splitgo
```

Migrations run automatically on startup. Without `SMTP_URL`, sign-in links are printed to the log.

| Variable       | Default | Description                                                                 |
| -------------- | ------- | --------------------------------------------------------------------------- |
| `DATABASE_DIR` | —       | Directory for `splitgo.sqlite` (required)                                   |
| `BASE_URL`     | —       | Public URL used in emailed links, e.g. `https://split.example.com` (required). Cookies are `Secure` when it is https. |
| `LISTEN_ADDR`  | `:8080` | HTTP listen address                                                         |
| `SMTP_URL`     | —       | `smtp://user:pass@host:587` (STARTTLS) or `smtps://user:pass@host:465`       |
| `MAIL_FROM`    | —       | Sender address, required when `SMTP_URL` is set                             |
| `ALLOW_SIGNUP` | `false` | Allow anyone to create an account. If disabled, only the first user and invited people can sign in. |

## Money handling

- Amounts are stored as `int64` minor units per currency, e.g. cents for EUR, yen for JPY, fils for KWD.
  SQL sums are therefore exact.
- User input is parsed with `shopspring/decimal` and must not use more decimal places than the currency has.
- The server computes every split (`split` package). The client only previews it, using the same code.
- Rounding uses the largest remainder method. Shares always add up to the total, and the payer absorbs
  any leftover cent, so nobody who owes money is charged an extra minor unit.
- Original split inputs (percentages, shares and so on) are stored, so editing shows exactly what was entered.
- Balances are derived from expenses on every read. A payment from A to B is stored as an expense paid by A
  in which B owes the full amount.
- Debt simplification (`balance.Simplify`) needs at most n−1 transfers and preserves every person's net balance.

## Development

```sh
make test   # tests (Ginkgo/Gomega)
make fuzz   # fuzz parsing, splitting and simplification
make lint   # go vet + revive
make dev    # live reload with air
make build  # static binary
```
