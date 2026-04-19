// Package transponder emits local operational telemetry to .leo/telemetry/.
//
// Telemetry is NEVER memory. Events are written to append-only JSONL files
// (.leo/telemetry/YYYY-MM-DD.jsonl, UTC day rotation) and are never indexed,
// recalled, or placed in .leo/memory/.
//
// Events are consumed by `leo doctor` (#70) and future Enterprise Dashboard.
// No network, no sync, no remote gateway — local only.
package transponder
