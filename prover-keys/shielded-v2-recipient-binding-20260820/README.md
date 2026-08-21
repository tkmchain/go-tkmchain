# Shielded V2 key bundle

These public artifacts match `TKM_SHIELDED_SPEND_V2` and activate on TKM
mainnet at `2026-08-27 12:00:00 UTC` (`1787832000`). Verify downloads with
`sha256sum -c SHA256SUMS` before use.

`proving.key` is downloaded automatically by `gtkm --tkmprover` and cached as
`proving-v2.key`. `verifying.key` is the gnark artifact and `verifying.hex` is
the `TKMG16VK1` chain encoding embedded in `params/shielded_privacy.go`.

The proving and verifying keys are public. Note openings, viewing keys, local
ML-DSA seed material, bearer tokens, and ceremony randomness are not.
