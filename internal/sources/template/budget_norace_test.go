//go:build !race

package template

// compileBudgetMs は TestCompileLargeBodyIsFast の上限（ミリ秒）。
const compileBudgetMs = 20
