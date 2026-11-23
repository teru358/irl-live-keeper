package i18n

// Codes は本体が出す文の一覧。
var Codes = []string{"fallback.scene", "fallback.target", "hidden.retained", "hidden.write", "hidden.load_failed", "hidden.load", "studio.ignore", "studio.hide", "studio.standby", "obs.legacy", "obs.switching", "obs.map", "obs.source_missing", "obs.legacy_url", "obs.media", "obs.scene_list", "obs.scene_missing", "switch.return", "switch.start_missing", "switch.unconfirmed", "switch.partial", "switch.failed", "switch.failed_detail", "switch.rejected", "switch.item_failed", "switch.restore_failed", "switch.persist_failed", "switch.no_target", "mediamtx.exit", "mediamtx.exit_error", "obs.slow_reader", "config.lan", "mediamtx.write", "mediamtx.start", "mediamtx.credentials", "mediamtx.port", "external.detail", "hidden.version", "hidden.entry"}

func init() {
	Codes = append(Codes, "purpose.auto_fail", "purpose.auto_recover", "purpose.standby_on", "purpose.standby_off_monitored", "purpose.standby_off_other", "purpose.start_scene", "purpose.start_stream", "purpose.stop_stream", "purpose.hide_camera", "purpose.show_camera", "purpose.restore_all", "config.validation_01", "config.validation_02", "config.validation_03", "config.validation_04", "config.validation_05", "config.validation_06", "config.validation_07", "config.validation_08", "config.validation_09", "config.validation_10", "config.validation_11", "config.validation_12", "config.validation_13", "config.validation_14", "config.validation_15", "config.validation_16", "config.validation_17", "config.validation_18", "config.validation_19", "config.validation_20", "config.validation_21", "config.validation_22", "config.validation_23", "config.validation_24", "config.validation_25", "config.validation_26", "config.validation_27", "config.validation_28", "config.validation_29", "config.validation_30", "config.validation_31", "config.validation_32", "config.validation_33", "config.validation_34")
}

func init() {
	Codes = append(Codes, "api.reason_streaming", "api.reason_not_streaming", "api.reason_already_standby", "api.reason_not_manual", "api.reason_same_pending", "api.reason_switch_in_process", "api.reason_standby_not_found", "api.reason_stopping", "api.reason_off_no_target", "api.reason_unknown", "api.not_found", "api.unauthorized", "api.method", "api.forbidden", "api.internal", "api.disconnected", "api.stopped", "api.deadline", "api.canceled", "api.invalid")
}
