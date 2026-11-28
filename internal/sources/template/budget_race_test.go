//go:build race

package template

// compileBudgetMs は TestCompileLargeBodyIsFast の上限（ミリ秒）。-race は数倍遅いので広げる。
// 前の2乗に近い作りは -race で 1 秒を超えるので、この上限でも後戻りを捕まえられる。
const compileBudgetMs = 200
