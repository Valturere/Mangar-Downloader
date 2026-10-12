# Skill: verify-change

Используй после изменения кода и перед commit/release. Основной источник истины — [AGENTS.md, раздел 14](../../../AGENTS.md).

1. По diff определить изменённые подсистемы и составить scope/risk-based verification plan.
2. Выполнить применимые baseline checks.
3. Глубоко проверить изменённое targeted tests; concurrency-sensitive Go packages — настоящим race detector (`scripts/verify-race.ps1`).
4. Для каждой cross-regression назвать пересекаемую boundary и возможный сбой.
5. Real/native/network checks запускать только по причине изменения или milestone. Destructive fixtures изолировать; production profile использовать лишь явно и non-destructive, сравнив identities/hashes до/после.
6. Проверить cleanup/orphans для затронутого cancellation/process пути.
7. Просмотреть diff на correctness/readability/architecture/security/performance, generated output, debug/TODO, секреты и неоговорённые изменения. Обновить документацию поведения.
8. Отчёт: Baseline, Targeted, Cross-regressions, Manual/real, Skipped as not relevant; перечислять только реально выполненные negative tests, ограничения и artifact path.

Обычный `scripts/verify.ps1` — baseline. UI/integrations/native/race включаются явными switches; `-Full` сохраняет широкий milestone regression. Не копировать прошлые checklist и не считать repeated tests заменой `-race`.
