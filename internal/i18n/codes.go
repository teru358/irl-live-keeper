package i18n

// Codes は本体が出す文の一覧。
var Codes = []string{"fallback.scene", "fallback.target", "hidden.retained", "hidden.write", "hidden.load_failed", "hidden.load", "studio.ignore", "studio.hide", "studio.standby", "obs.legacy", "obs.switching", "obs.map", "obs.source_missing", "obs.legacy_url", "obs.media", "obs.scene_list", "obs.scene_missing", "switch.return", "switch.start_missing", "switch.unconfirmed", "switch.partial", "switch.failed", "switch.failed_detail", "switch.rejected", "switch.item_failed", "switch.restore_failed", "switch.persist_failed", "switch.no_target", "mediamtx.exit", "mediamtx.exit_error", "obs.slow_reader", "config.lan", "mediamtx.write", "mediamtx.start", "mediamtx.credentials", "mediamtx.port", "external.detail", "hidden.version", "hidden.entry"}

func init() {
	Codes = append(Codes, "purpose.auto_fail", "purpose.auto_recover", "purpose.standby_on", "purpose.standby_off_monitored", "purpose.standby_off_other", "purpose.start_scene", "purpose.start_stream", "purpose.stop_stream", "purpose.hide_camera", "purpose.show_camera", "purpose.restore_all", "config.validation_01", "config.validation_02", "config.validation_03", "config.validation_04", "config.validation_05", "config.validation_06", "config.validation_07", "config.validation_08", "config.validation_09", "config.validation_10", "config.validation_11", "config.validation_12", "config.validation_13", "config.validation_14", "config.validation_15", "config.validation_16", "config.validation_17", "config.validation_18", "config.validation_19", "config.validation_20", "config.validation_21", "config.validation_23", "config.validation_24", "config.validation_25", "config.validation_26", "config.validation_27", "config.validation_28", "config.validation_29", "config.validation_30", "config.validation_31", "config.validation_32", "config.validation_33", "config.validation_34")
}

func init() {
	Codes = append(Codes, "api.reason_streaming", "api.reason_not_streaming", "api.reason_already_standby", "api.reason_not_manual", "api.reason_same_pending", "api.reason_switch_in_process", "api.reason_standby_not_found", "api.reason_stopping", "api.reason_off_no_target", "api.reason_unknown", "api.not_found", "api.unauthorized", "api.too_many_attempts", "api.method", "api.forbidden", "api.internal", "api.disconnected", "api.stopped", "api.deadline", "api.canceled", "api.invalid")
	Codes = append(Codes, "banner.title", "banner.publish", "banner.host_missing", "banner.outdoor", "banner.obs", "banner.admin", "banner.admin_user", "banner.password", "banner.camera_label", "banner.lan_note", "banner.lan_missing", "banner.sensitive_note", "console.logger_open", "console.port_check", "console.listen", "console.serve", "console.config_load", "console.config_invalid", "console.executable", "console.pause", "console.rollback")
	Codes = append(Codes, "service.root", "service.root_user", "service.write_error", "service.missing", "service.install_user", "service.install_system", "service.uninstall_user", "service.uninstall_system", "service.url", "service.lan", "service.vpn", "service.linger", "service.usage", "service.linux_only", "service.need_value", "service.unknown_arg")
}

func init() {
	Codes = append(Codes, "api.scene_missing", "api.scene_failed", "settings.invalid", "settings.applying", "settings.hash_conflict", "settings.local_only", "settings.admin_lan_close", "settings.port_missing", "settings.port_busy", "settings.admin_password_unset", "app.settings_apply_failed", "app.obs_url_pending", "app.obs_url_missing")
}

func init() { Codes = append(Codes, "mediamtx.missing") }

func init() { Codes = append(Codes, "phone.lan_disabled", "phone.invalid_host") }

func init() {
	Codes = append(Codes, "template.syntax", "template.unknown", "template.format", "template.comparison")
}

func init() {
	Codes = append(Codes, "sources.invalid", "sources.reserved_name", "sources.body_too_large", "sources.unreadable", "sources.duplicate", "sources.write_failed")
}

func init() { Codes = append(Codes, "sources.render_failed") }

func init() {
	Codes = append(Codes, "source_files.invalid", "source_files.name_too_long", "source_files.duplicate", "source_files.limit", "source_files.in_use", "source_files.io", "template.asset_missing", "template.style_literal")
}

func init() {
	Codes = append(Codes, "sources.test_streaming", "sources.test_stream_started", "sources.test_timeout")
}

func init() {
	Codes = append(Codes, "source_files.reason_name_collision", "source_files.reason_bad_name", "source_files.reason_reserved_name", "source_files.reason_bad_extension", "source_files.reason_path_too_long", "source_files.reason_too_deep", "source_files.reason_too_large", "source_files.reason_not_image", "source_files.reason_ext_mismatch", "source_files.reason_symlink", "source_files.reason_not_regular", "source_files.reason_unreadable", "source_files.reason_over_limit", "source_files.scan_limit")
}

func init() {
	Codes = append(Codes, "template.html_lex", "template.html_comment", "template.html_style_unclosed", "template.html_nul", "template.html_element", "template.html_attribute", "template.html_duplicate", "template.html_quote", "template.html_nesting", "template.html_branch", "template.html_template_position", "template.html_style_type", "template.html_url", "template.css_escape", "template.css_import", "template.css_function", "template.css_syntax")
}

// ログインの失敗（パスワードが違う）の文。ログインが切れたときの api.unauthorized とは別にする。
func init() { Codes = append(Codes, "api.wrong_password") }
