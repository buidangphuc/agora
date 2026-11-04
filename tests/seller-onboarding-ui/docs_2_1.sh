#!/usr/bin/env bash
cd "$(dirname "$0")/../.." || exit 2
fail=0
if grep -n 'G15' fe-platform/docs/gaps.md; then echo "FAIL seller-onboarding-ui: G15 row still in gaps.md"; fail=1; fi
if ! grep -q 'SELLER_TERMS_VERSION' team-frontend-seller/README.md; then echo "FAIL seller-onboarding-ui: SELLER_TERMS_VERSION not documented"; fail=1; fi
exit $fail
