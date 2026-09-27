#!/usr/bin/env python3
import argparse
import base64
import json
import pathlib
import sys
import time


def fail(message: str) -> None:
    raise SystemExit(message)


def positive_int(value: object, name: str) -> int:
    if isinstance(value, bool) or not isinstance(value, int) or value <= 0:
        fail(f"invalid OIDC {name}")
    return value


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Validate the non-secret identity facts of one GitHub Actions OIDC assertion."
    )
    parser.add_argument("--token-file", required=True)
    parser.add_argument("--audience", required=True)
    parser.add_argument("--repository", required=True)
    parser.add_argument("--repository-id", required=True)
    parser.add_argument("--repository-owner-id", required=True)
    parser.add_argument("--ref", required=True)
    parser.add_argument("--workflow-ref", required=True)
    parser.add_argument("--max-lifetime-seconds", type=int, default=600)
    parser.add_argument("--min-remaining-seconds", type=int, default=120)
    args = parser.parse_args()

    if args.max_lifetime_seconds <= 0 or args.min_remaining_seconds <= 0:
        fail("OIDC lifetime bounds must be positive")

    token = pathlib.Path(args.token_file).read_text(encoding="utf-8")
    parts = token.split(".")
    if len(parts) != 3:
        fail("OIDC token is not a compact JWT")
    try:
        payload = parts[1] + "=" * (-len(parts[1]) % 4)
        claims = json.loads(base64.urlsafe_b64decode(payload))
    except Exception as exc:
        fail(f"OIDC payload is invalid: {exc}")
    if not isinstance(claims, dict):
        fail("OIDC payload is not an object")

    if claims.get("iss") != "https://token.actions.githubusercontent.com":
        fail("unexpected OIDC issuer")
    aud = claims.get("aud")
    if not (aud == args.audience or isinstance(aud, list) and args.audience in aud):
        fail("unexpected OIDC audience")
    if claims.get("repository") != args.repository or claims.get("ref") != args.ref:
        fail("unexpected repository/ref claims")
    if str(claims.get("repository_id", "")) != args.repository_id:
        fail("unexpected repository_id")
    if str(claims.get("repository_owner_id", "")) != args.repository_owner_id:
        fail("unexpected repository_owner_id")
    if claims.get("workflow_ref") != args.workflow_ref:
        fail("unexpected workflow_ref")

    jti = claims.get("jti")
    if not isinstance(jti, str) or not jti.strip():
        fail("non-empty GitHub OIDC jti required for replay protection")

    issued_at = positive_int(claims.get("iat"), "iat")
    expires_at = positive_int(claims.get("exp"), "exp")
    if expires_at <= issued_at:
        fail("OIDC exp must be after iat")
    lifetime = expires_at - issued_at
    if lifetime > args.max_lifetime_seconds:
        fail("OIDC assertion lifetime exceeds configured provider maximum")

    now = int(time.time())
    remaining = expires_at - now
    if remaining < args.min_remaining_seconds:
        fail("OIDC assertion has insufficient remaining lifetime")

    nbf = claims.get("nbf")
    if nbf is not None:
        not_before = positive_int(nbf, "nbf")
        if not_before > now + 30:
            fail("OIDC assertion is not yet valid")

    # GitHub changed the default subject format for newer repositories in 2026.
    # Trust is intentionally bound to immutable repository IDs plus exact
    # repository/ref/workflow claims, not to a mutable/legacy sub string shape.
    subject = claims.get("sub")
    if not isinstance(subject, str) or not subject:
        fail("non-empty OIDC subject required")


if __name__ == "__main__":
    main()
