package bot

// printJSON 是 bot service 的内部诊断输出钩子；当前为 noop，与重构前
// main.go 中的 printJSON 行为一致（注释掉了实际写出）。
// 如需调试可直接替换实现。
func printJSON(record map[string]any) {
	_ = record
}
