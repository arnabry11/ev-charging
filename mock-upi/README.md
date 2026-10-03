# Mock UPI

A fake prepaid payment provider. Run it from the repository root:

```bash
docker compose up --build
```

- `POST /payments` creates a pending payment. The same `idempotency_key` and body return the original payment.
- `POST /payments/{id}/succeed` and `POST /payments/{id}/fail` send one signed webhook.
- `POST /payments/{id}/replay-webhook` sends that same webhook again.

The webhook signature is `X-Signature = hex(HMAC_SHA256(WEBHOOK_SECRET, timestamp + "." + raw_body))`.
