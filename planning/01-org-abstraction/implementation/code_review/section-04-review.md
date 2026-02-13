# Section 04 Code Review: Analyzer

## CRITICAL
- C-1: Race on context cancellation early return — goroutines still running when result returned
- C-2: Total invariant broken — cancelled goroutines don't update any counter

## MEDIUM
- M-1: Plan step 6b "Get effective config" not implemented
- M-2: Skipped field never populated
- M-3: isNonRetryable uses fragile string matching
- M-4: time.After creates timer leak on ctx cancel

## LOW
- L-1: forceFlag tracking is global not per-repo
- L-2: ClampsConcurrency test doesn't verify actual clamped values
- L-3: Context cancellation test is timing-dependent

## COSMETIC
- CO-1: analyzeRepo accepts unused orgID parameter
- CO-2: No logging in analyzer
