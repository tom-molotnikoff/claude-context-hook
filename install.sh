#!/bin/bash
set -euo pipefail
dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
claude plugin marketplace add "$dir"
claude plugin install ctx@ctx
claude plugin update ctx@ctx
