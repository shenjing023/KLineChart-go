# Evidence cleanup (2026-10-22)

## Kept

The final validation set is:

- `2026-10-21-final-tests.txt`
- `2026-10-21-final-shuffle.txt`
- `2026-10-21-final-race.txt`
- `2026-10-21-final-examples.txt`
- `2026-10-21-final-coverage` and `2026-10-21-final-coverage.txt`
- `2026-10-21-final-contract-tests.txt`
- `2026-10-21-final-bench-refreshed.txt`
- `2026-10-21-final-launch.txt`
- `2026-10-21-final-window-acceptance.md`
- `2026-10-22-final-interaction.txt`

The following are retained as prior-round evidence because the final
acceptance record explicitly distinguishes them from the current build:
`2026-10-21-history.png`, `2026-10-21-host-window.png`,
`2026-10-21-indicator.png`, `2026-10-21-live1.png`,
`2026-10-21-paint.png`, and `2026-10-21-trades.png`.

## Removed

- `2026-10-21-final-bench.txt` was superseded by the refreshed final run.
- `2026-10-21-extend-output.txt` duplicated the reproducible `-headless` smoke.
- `2026-10-21-replay-output.txt` duplicated the reproducible `-headless` smoke.

The six historical PNGs above were not deleted: they are cited as earlier-build
evidence, and deleting them would make the acceptance record's distinction
between prior and current evidence harder to audit.
