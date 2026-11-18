#!/usr/bin/env bash
set -euo pipefail

# A local command fixture for exercising worktrees, logs and checks.
# It does not interpret the task or invoke any AI model.
cat > eng-fixture-input.txt
printf 'Local eng fixture output. No model or device was used.\n' > eng-fixture-output.txt
printf 'Created eng-fixture-input.txt and eng-fixture-output.txt in %s\n' "$PWD"
