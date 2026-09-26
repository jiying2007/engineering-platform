# Codex WIF Admin API helper

This helper removes the manual OpenAI Admin Portal step **after Codex workload
identity federation has been enabled for the managed ChatGPT workspace**.

OpenAI's Codex WIF Admin API requires:

- an Admin API key whose active owner may manage workload identity;
- the managed ChatGPT workspace ID;
- an existing active ChatGPT user/service-account OpenAI user ID;
- the GitHub OIDC audience.

Run this only on an administrator-controlled workstation. Do **not** put
`OPENAI_ADMIN_KEY` in repository variables/secrets and do not pass it to
Codex, Worker, or GitHub-hosted retained-pilot workflows.

## Create or verify the provider/rule

```sh
export OPENAI_ADMIN_KEY='<admin-api-key>'
export WORKSPACE_ID='<managed-chatgpt-workspace-id>'
export PRINCIPAL_ID='<existing-active-openai-user-or-service-account-id>'
export OPENAI_WIF_AUDIENCE='<dedicated-github-oidc-audience>'

bash examples/pilots/wif/configure-admin-api.sh /operator/wif-admin
```

The helper creates only when the named resource is absent. If a same-named
provider/rule already exists but its trust policy differs, it fails closed rather
than silently broadening or rewriting administrator policy.

The provider is bound to GitHub Actions OIDC issuer
`https://token.actions.githubusercontent.com`, the dedicated audience,
replay checking, and a 600-second assertion lifetime.

The rule requires:

- exact repository `jiying2007/engineering-platform`;
- exact ref `refs/heads/main`;
- the dedicated audience;
- a CEL allow-list containing only:
  - `.github/workflows/codex-wif-live.yml@refs/heads/main`;
  - `.github/workflows/retained-pilot-engineer.yml@refs/heads/main`;
- a 600-second OpenAI access-token lifetime.

To also write the two non-secret GitHub Actions repository variables from the
admin workstation, set:

```sh
export SET_GITHUB_VARIABLES=1
```

The generated `wif-admin-receipt.json` contains provider/rule IDs and the
non-secret trust policy. It never contains the Admin API key.

If the Admin API returns 403/404 because the organization/workspace is not
enabled for Codex WIF beta or the key owner lacks permission, that remains the
one external enablement boundary; do not substitute an API key or stored ChatGPT
login for the retained Codex runtime.
