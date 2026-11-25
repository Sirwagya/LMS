use crate::PushItemOutcome;
use std::collections::HashMap;

pub fn compute_backoff_ms(retry_count: u32, jitter_unit: f64) -> u64 {
    let capped = (1_000_u64 << retry_count.min(9)).min(300_000);
    (capped as f64 * (0.5 + 0.5 * jitter_unit.clamp(0.0, 1.0))) as u64
}

pub(crate) fn acknowledge_by_key(
    outcomes: &[PushItemOutcome],
) -> Result<HashMap<&str, &PushItemOutcome>, crate::SyncError> {
    let mut indexed = HashMap::new();
    for outcome in outcomes {
        if let Some(previous) = indexed.insert(outcome.idempotency_key.as_str(), outcome)
            && (previous.status != outcome.status
                || previous.server_version != outcome.server_version
                || previous.server_payload != outcome.server_payload
                || previous.reason != outcome.reason)
        {
            return Err(crate::SyncError::Protocol(
                "conflicting acknowledgements for a mutation".into(),
            ));
        }
    }
    Ok(indexed)
}
