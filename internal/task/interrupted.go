package task

import "errors"

// InterruptedError 标记"已经开始以后被中断"的直播任务（推流 / 屏幕推流已经在输出，之后进程被杀、服务器断开、源没了）：
// 终态记为 interrupted 而不是 failed，error 照常记录（契约 v0.25.3，和直播页显示的"被中断"一致）。
// 只对直播任务生效；其他类型的 Runner 返回它时按普通失败处理。
type InterruptedError struct{ Err error }

func (e *InterruptedError) Error() string { return e.Err.Error() }
func (e *InterruptedError) Unwrap() error { return e.Err }

// Interrupted 把 err 包成 InterruptedError；err 为 nil 时返回 nil。
func Interrupted(err error) error {
	if err == nil {
		return nil
	}
	return &InterruptedError{Err: err}
}

// IsInterrupted 判断 err 链上有没有 InterruptedError。
func IsInterrupted(err error) bool {
	var ie *InterruptedError
	return errors.As(err, &ie)
}
