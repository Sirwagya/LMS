use std::time::Duration;

use async_trait::async_trait;
use reqwest::{Client, RequestBuilder, Url, redirect::Policy};
use serde::{Deserialize, Serialize, de::DeserializeOwned};
use serde_json::Value;

use crate::projection::invalid;
use crate::{OutboxMutation, PullResponse, PushItemOutcome, RemoteSyncPort, SyncError};

pub struct HttpRemote {
    base: Url,
    client: Client,
}

impl HttpRemote {
    pub fn new(base: &str) -> Result<Self, SyncError> {
        let base = Url::parse(base).map_err(|_| invalid("invalid API base URL"))?;
        if !matches!(base.scheme(), "http" | "https")
            || base.host().is_none()
            || !base.username().is_empty()
            || base.password().is_some()
            || base.query().is_some()
            || base.fragment().is_some()
        {
            return Err(invalid("invalid API base URL"));
        }
        let client = Client::builder()
            .connect_timeout(Duration::from_secs(5))
            .timeout(Duration::from_secs(30))
            .redirect(Policy::none())
            .build()
            .map_err(network_error)?;
        Ok(Self { base, client })
    }

    fn url(&self, path: &str) -> Result<Url, SyncError> {
        self.base
            .join(path)
            .map_err(|_| invalid("invalid API path"))
    }
}

fn network_error(error: reqwest::Error) -> SyncError {
    if error.is_builder() || error.is_decode() {
        return invalid("invalid HTTP request or response");
    }
    SyncError::Network(error.without_url().to_string())
}

async fn receive<T: DeserializeOwned>(request: RequestBuilder) -> Result<T, SyncError> {
    let response = request.send().await.map_err(network_error)?;
    let status = response.status();
    if status.as_u16() == 401 {
        return Err(SyncError::Unauthorized);
    }
    if !status.is_success() {
        let retry_after_ms = response
            .headers()
            .get(reqwest::header::RETRY_AFTER)
            .and_then(|header| header.to_str().ok())
            .and_then(|seconds| seconds.parse::<u64>().ok())
            .map(|seconds| seconds.saturating_mul(1_000));
        return Err(SyncError::Http {
            status: status.as_u16(),
            retry_after_ms,
        });
    }
    response.json::<T>().await.map_err(network_error)
}

// Keep local endpoint bookkeeping and retry counters out of the Go request.
// Replaying an item with a different wire payload would invalidate its key.
#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct WireMutation<'a> {
    idempotency_key: &'a str,
    entity_type: &'a str,
    entity_id: &'a str,
    base_version: i64,
    payload: &'a Value,
}

#[derive(Serialize)]
struct PushRequest<'a> {
    mutations: Vec<WireMutation<'a>>,
}

#[derive(Deserialize)]
struct PushResponse {
    results: Vec<PushItemOutcome>,
}

#[async_trait]
impl RemoteSyncPort for HttpRemote {
    async fn fetch_bootstrap(&self, bearer_token: &str) -> Result<Value, SyncError> {
        receive(
            self.client
                .get(self.url("/api/v1/bootstrap")?)
                .bearer_auth(bearer_token),
        )
        .await
    }

    async fn push_mutations(
        &self,
        bearer_token: &str,
        batch: &[OutboxMutation],
    ) -> Result<Vec<PushItemOutcome>, SyncError> {
        if batch.is_empty() {
            return Ok(Vec::new());
        }
        let request = PushRequest {
            mutations: batch
                .iter()
                .map(|item| WireMutation {
                    idempotency_key: &item.idempotency_key,
                    entity_type: &item.entity_type,
                    entity_id: &item.entity_id,
                    base_version: item.base_version,
                    payload: &item.payload_json,
                })
                .collect(),
        };
        let response: PushResponse = receive(
            self.client
                .post(self.url("/api/v1/sync/push")?)
                .bearer_auth(bearer_token)
                .json(&request),
        )
        .await?;
        Ok(response.results)
    }

    async fn pull_delta(
        &self,
        bearer_token: &str,
        cursor: i64,
        limit: usize,
    ) -> Result<PullResponse, SyncError> {
        receive(
            self.client
                .get(self.url("/api/v1/sync/pull")?)
                .bearer_auth(bearer_token)
                .query(&[("cursor", cursor), ("limit", limit.clamp(1, 200) as i64)]),
        )
        .await
    }
}
