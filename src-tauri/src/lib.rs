mod commands;
pub mod keychain;
mod proctor;

use std::sync::Arc;

use tauri::{
    AppHandle, Manager,
    menu::{Menu, MenuBuilder, MenuItemBuilder, SubmenuBuilder},
};

use crate::commands::{
    DesktopCore, EventSink, TauriEventSink, lms_export_asset, lms_load_code_draft, lms_request,
    lms_save_code_draft, lms_sync_now, lms_sync_status,
};
use crate::keychain::{MacOSKeychainVault, SessionVault};
use crate::proctor::{
    lms_proctor_capture_screen, lms_proctor_get_fullscreen, lms_proctor_list_apps,
    lms_proctor_lockdown, lms_proctor_quit_other_apps, lms_proctor_set_fullscreen,
};

fn build_app_menu(app: &AppHandle) -> tauri::Result<Menu<tauri::Wry>> {
    let settings = MenuItemBuilder::with_id("open_settings", "Account & Settings…")
        .accelerator("CmdOrCtrl+,")
        .build(app)?;

    let app_submenu = SubmenuBuilder::new(app, "Vedam LMS")
        .about(None)
        .separator()
        .item(&settings)
        .separator()
        .services()
        .separator()
        .hide()
        .hide_others()
        .show_all()
        .separator()
        .quit()
        .build()?;

    let edit_submenu = SubmenuBuilder::new(app, "Edit")
        .undo()
        .redo()
        .separator()
        .cut()
        .copy()
        .paste()
        .select_all()
        .build()?;

    let nav_dashboard = MenuItemBuilder::with_id("nav_dashboard", "Dashboard")
        .accelerator("CmdOrCtrl+1")
        .build(app)?;
    let nav_overview = MenuItemBuilder::with_id("nav_overview", "Attendance & Results")
        .accelerator("CmdOrCtrl+2")
        .build(app)?;
    let nav_assignments = MenuItemBuilder::with_id("nav_assignments", "Assignments")
        .accelerator("CmdOrCtrl+3")
        .build(app)?;
    let nav_schedule = MenuItemBuilder::with_id("nav_schedule", "Timetable")
        .accelerator("CmdOrCtrl+4")
        .build(app)?;
    let nav_fees = MenuItemBuilder::with_id("nav_fees", "Fees")
        .accelerator("CmdOrCtrl+5")
        .build(app)?;
    let nav_account = MenuItemBuilder::with_id("nav_account", "Account")
        .accelerator("CmdOrCtrl+6")
        .build(app)?;
    let sync_now = MenuItemBuilder::with_id("sync_now", "Sync Workspace Now")
        .accelerator("CmdOrCtrl+Shift+S")
        .build(app)?;

    let view_submenu = SubmenuBuilder::new(app, "Workspace")
        .item(&nav_dashboard)
        .item(&nav_overview)
        .item(&nav_assignments)
        .item(&nav_schedule)
        .item(&nav_fees)
        .item(&nav_account)
        .separator()
        .item(&sync_now)
        .build()?;

    let window_submenu = SubmenuBuilder::new(app, "Window")
        .minimize()
        .maximize()
        .separator()
        .close_window()
        .build()?;

    MenuBuilder::new(app)
        .item(&app_submenu)
        .item(&edit_submenu)
        .item(&view_submenu)
        .item(&window_submenu)
        .build()
}

pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_notification::init())
        .plugin(tauri_plugin_opener::init())
        .setup(|app| {
            let data_dir = app.path().app_data_dir()?;
            let vault: Arc<dyn SessionVault> = Arc::new(MacOSKeychainVault::default());
            let events: Arc<dyn EventSink> = Arc::new(TauriEventSink::new(app.handle().clone()));
            let core = Arc::new(
                DesktopCore::new(data_dir, DesktopCore::resolved_api_url(), vault, events)
                    .map_err(std::io::Error::other)?,
            );
            app.manage(Arc::clone(&core));

            let core_resume = Arc::clone(&core);
            tauri::async_runtime::spawn(async move {
                core_resume.resume_persisted_session().await;
            });

            let menu = build_app_menu(app.handle())?;
            app.set_menu(menu)?;

            let core_menu = Arc::clone(&core);
            app.on_menu_event(move |_app, event| {
                let hash = match event.id().as_ref() {
                    "nav_dashboard" => Some("#/dashboard"),
                    "nav_overview" => Some("#/overview"),
                    "nav_assignments" => Some("#/assignments"),
                    "nav_schedule" => Some("#/schedule"),
                    "nav_fees" => Some("#/fees"),
                    "nav_account" | "open_settings" => Some("#/account"),
                    "sync_now" => {
                        let core_sync = Arc::clone(&core_menu);
                        tauri::async_runtime::spawn(async move {
                            if let Ok(status) = core_sync.sync_now().await {
                                core_sync.events().emit_sync_updated(&status);
                                core_sync
                                    .events()
                                    .emit_server_event("sync.available", &serde_json::json!({}));
                            }
                        });
                        None
                    }
                    _ => None,
                };
                if let Some(target) = hash {
                    core_menu.events().emit_navigate(target);
                }
            });

            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            lms_request,
            lms_sync_now,
            lms_sync_status,
            lms_save_code_draft,
            lms_load_code_draft,
            lms_export_asset,
            lms_proctor_list_apps,
            lms_proctor_lockdown,
            lms_proctor_quit_other_apps,
            lms_proctor_set_fullscreen,
            lms_proctor_get_fullscreen,
            lms_proctor_capture_screen,
        ])
        .run(tauri::generate_context!())
        .expect("error while running Vedam LMS desktop shell");
}
