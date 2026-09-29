# Managed-workspace WIF qualification helper

This helper starts **after** the ChatGPT workspace administrator has created the
managed-workspace Codex WIF provider/rule.

It does not create the OpenAI-side provider or rule. Issue #103 tracks that
external administrator gate. After the provider/rule exists, this helper
automates the remaining GitHub-side qualification steps:

1. set the two repository Actions variables;
2. verify local main == remote main;
3. dispatch `codex-wif-live.yml` on protected main;
4. identify and watch the exact dispatched run;
5. download both the retained live-turn receipt and live compatibility
   qualification;
6. require the live compatibility qualification to equal the frozen
   `codex-profile.json` qualification exactly, including exact native binary
   digest/version/model contract;
7. require the live receipt to bind that same binary digest plus exact
   model/federation rule and no approval/tool effects;
8. retain both receipts locally and render `worker-codex.json` from the exact
   existing `codex-profile.json`, including both the frozen compatibility
   `qualification` and `profile` required by Worker pre-execution
   requalification.

Example:

```sh
bash examples/pilots/wif/qualify.sh \
  '<administrator-configured-audience>' \
  '<administrator-provided-federation-rule-id>' \
  /operator/codex-profile.json \
  /operator/wif
```

After success, retain both:

- `/operator/wif/codex-wif-live-receipt.json`;
- `/operator/wif/codex-compatibility-qualification.json`.

Then the existing pilot preflight may consume the WIF receipt:

```sh
eng pilot-preflight \
  --repository /absolute/operator/checkout/engineering-platform \
  --base "$BASE_COMMIT" \
  --codex-profile /operator/codex-profile.json \
  --access-policy /operator/access-policy.json \
  --preparation /operator/worker-preparation.json \
  --worker-profile worker/codex-pilot \
  --worker-codex /operator/wif/worker-codex.json \
  --wif-receipt /operator/wif/codex-wif-live-receipt.json
```

Add `--publisher` only after the independent publisher credential has also
been provisioned.

The helper never prints or downloads the GitHub OIDC assertion itself. That
assertion remains inside the protected workflow runner and is destroyed by the
workflow after the live turn.
