use std::sync::Mutex;

use security_framework::passwords::{
    delete_generic_password, get_generic_password, set_generic_password,
};
use serde::{Deserialize, Serialize};

const DEFAULT_SERVICE: &str = "com.vedam.lms";
const DEFAULT_ACCOUNT: &str = "active_session_token";

#[derive(Clone, Debug, Deserialize, Serialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct StoredSession {
    pub user_id: String,
    pub role: String,
    pub token: String,
}

/// Port for persisting active user credentials outside WebView storage.
pub trait SessionVault: Send + Sync + 'static {
    fn save(&self, session: &StoredSession) -> Result<(), String>;
    fn load(&self) -> Option<StoredSession>;
    fn clear(&self) -> Result<(), String>;
}

/// macOS Keychain-backed credential store using Security.framework.
pub struct MacOSKeychainVault {
    service: String,
    account: String,
    cache: Mutex<Option<Option<StoredSession>>>,
}

impl Default for MacOSKeychainVault {
    fn default() -> Self {
        Self::new(DEFAULT_SERVICE, DEFAULT_ACCOUNT)
    }
}

impl MacOSKeychainVault {
    pub fn new(service: impl Into<String>, account: impl Into<String>) -> Self {
        Self {
            service: service.into(),
            account: account.into(),
            cache: Mutex::new(None),
        }
    }

    fn read_from_keychain(&self) -> Option<StoredSession> {
        let bytes = get_generic_password(&self.service, &self.account).ok()?;
        serde_json::from_slice::<StoredSession>(&bytes).ok()
    }
}

impl SessionVault for MacOSKeychainVault {
    fn save(&self, session: &StoredSession) -> Result<(), String> {
        let mut cache = self.cache.lock().map_err(|_| "vault lock poisoned")?;
        let encoded = serde_json::to_vec(session).map_err(|e| e.to_string())?;
        set_generic_password(&self.service, &self.account, &encoded).map_err(|e| e.to_string())?;
        *cache = Some(Some(session.clone()));
        Ok(())
    }

    fn load(&self) -> Option<StoredSession> {
        let mut cache = self.cache.lock().ok()?;
        if cache.is_none() {
            *cache = Some(self.read_from_keychain());
        }
        cache.as_ref().cloned().flatten()
    }

    fn clear(&self) -> Result<(), String> {
        const ERR_ITEM_NOT_FOUND: i32 = -25300;
        let mut cache = self.cache.lock().map_err(|_| "vault lock poisoned")?;
        *cache = Some(None);
        match delete_generic_password(&self.service, &self.account) {
            Ok(()) => Ok(()),
            Err(err) if err.code() == ERR_ITEM_NOT_FOUND => Ok(()),
            Err(err) => Err(format!("Could not delete the session from Keychain: {err}")),
        }
    }
}

/// Ephemeral in-memory vault used for deterministic automated tests.
#[derive(Default)]
pub struct MemorySessionVault {
    inner: Mutex<Option<StoredSession>>,
}

impl SessionVault for MemorySessionVault {
    fn save(&self, session: &StoredSession) -> Result<(), String> {
        let mut guard = self.inner.lock().map_err(|_| "vault lock poisoned")?;
        *guard = Some(session.clone());
        Ok(())
    }

    fn load(&self) -> Option<StoredSession> {
        self.inner.lock().ok().and_then(|g| g.clone())
    }

    fn clear(&self) -> Result<(), String> {
        *self.inner.lock().map_err(|_| "vault lock poisoned")? = None;
        Ok(())
    }
}

pub fn save_session_token(token: &str) -> Result<(), String> {
    set_generic_password(DEFAULT_SERVICE, DEFAULT_ACCOUNT, token.as_bytes())
        .map_err(|e| e.to_string())
}

pub fn load_session_token() -> Option<String> {
    get_generic_password(DEFAULT_SERVICE, DEFAULT_ACCOUNT)
        .ok()
        .and_then(|bytes| String::from_utf8(bytes).ok())
}

pub fn clear_session_token() {
    let _ = delete_generic_password(DEFAULT_SERVICE, DEFAULT_ACCOUNT);
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn memory_vault_round_trip() {
        let vault = MemorySessionVault::default();
        assert_eq!(vault.load(), None);
        let session = StoredSession {
            user_id: "student-1".into(),
            role: "student".into(),
            token: "tok-secret-123".into(),
        };
        vault.save(&session).unwrap();
        assert_eq!(vault.load(), Some(session));
        vault.clear().unwrap();
        assert_eq!(vault.load(), None);
    }

    #[test]
    fn macos_keychain_vault_round_trip_in_isolated_service() {
        let service = format!("com.vedam.lms.test.{}", uuid::Uuid::now_v7());
        let vault = MacOSKeychainVault::new(service, "test_account");
        let session = StoredSession {
            user_id: "user-abc".into(),
            role: "student".into(),
            token: "bearer-token-xyz".into(),
        };
        match vault.save(&session) {
            Ok(()) => {
                // Re-read using a fresh instance without in-memory cache.
                let fresh = MacOSKeychainVault::new(&vault.service, &vault.account);
                assert_eq!(fresh.load(), Some(session));
                fresh.clear().unwrap();
                let after_clear = MacOSKeychainVault::new(&vault.service, &vault.account);
                assert_eq!(after_clear.load(), None);
            }
            Err(err) => {
                // Headless CI sessions without an unlocked login keychain return errSecInteractionNotAllowed.
                assert!(
                    err.contains("interaction") || err.contains("-25308"),
                    "unexpected keychain error: {err}"
                );
            }
        }
    }
}
