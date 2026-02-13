# Code Review Interview: Section 02 - Config Inheritance

## Auto-fixes
1. Strengthen mutation test to use element mutation instead of append
2. Add symmetric nil-org pointer test

## Let go
- Negative MaxFileSize validation (out of scope)
- nil-vs-empty ExcludePatterns distinction (behavior is correct)
- Doc comments on helpers (minimal code, self-explanatory)
