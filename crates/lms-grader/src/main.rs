use std::io::{Read, Write};

use lms_grader::{GradeRequest, IpcEnvelope, JudgePipeline};

#[tokio::main(flavor = "current_thread")]
async fn main() {
    const MAX_REQUEST: u64 = 5 << 20;
    let mut raw = String::new();
    if let Err(err) = std::io::stdin()
        .take(MAX_REQUEST + 1)
        .read_to_string(&mut raw)
    {
        write_envelope(IpcEnvelope::err(
            500,
            format!("Failed to read input: {err}"),
        ));
        return;
    }

    if raw.len() as u64 > MAX_REQUEST {
        write_envelope(IpcEnvelope::err(400, "Grader request is too large."));
        return;
    }
    let request: GradeRequest = match serde_json::from_str(&raw) {
        Ok(req) => req,
        Err(err) => {
            write_envelope(IpcEnvelope::err(
                400,
                format!("Invalid grader request: {err}"),
            ));
            return;
        }
    };

    let mut term = match tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate()) {
        Ok(signal) => signal,
        Err(_) => {
            write_envelope(IpcEnvelope::err(
                503,
                "Could not initialize grader shutdown.",
            ));
            return;
        }
    };
    let judge = JudgePipeline::default();
    let result = tokio::select! {
        biased;
        _=term.recv()=>return,
        result=judge.evaluate(request)=>result,
    };
    let envelope = match result {
        Ok(result) => IpcEnvelope::ok(result),
        Err(err) => IpcEnvelope::err(err.status_code(), err.to_string()),
    };

    write_envelope(envelope);
}

fn write_envelope(envelope: IpcEnvelope) {
    let payload = serde_json::to_string(&envelope)
        .unwrap_or_else(|_| r#"{"status":500,"message":"Serialize failure."}"#.to_string());
    let mut stdout = std::io::stdout().lock();
    let _ = stdout.write_all(payload.as_bytes());
    let _ = stdout.flush();
}
