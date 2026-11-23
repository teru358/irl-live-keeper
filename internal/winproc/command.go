package winproc

import "strings"

func URLCommand(url string) (string, []string) {
	return "rundll32.exe", []string{"rundll32.exe", "url.dll,FileProtocolHandler", url}
}

func FolderCommand(path string) (string, []string) {
	return "explorer.exe", []string{"explorer.exe", path}
}

// WindowsCommandLine は Windows の CreateProcess 用に argv を引用して連結する。
func WindowsCommandLine(args ...string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = quoteWindowsArg(arg)
	}
	return strings.Join(quoted, " ")
}

func quoteWindowsArg(arg string) string {
	if arg != "" && !strings.ContainsAny(arg, " \t\n\v\"") {
		return arg
	}
	var b strings.Builder
	b.WriteByte('"')
	backslashes := 0
	for _, r := range arg {
		if r == '\\' {
			backslashes++
			continue
		}
		if r == '"' {
			b.WriteString(strings.Repeat(`\`, backslashes*2+1))
			b.WriteRune(r)
			backslashes = 0
			continue
		}
		b.WriteString(strings.Repeat(`\`, backslashes))
		backslashes = 0
		b.WriteRune(r)
	}
	b.WriteString(strings.Repeat(`\`, backslashes*2))
	b.WriteByte('"')
	return b.String()
}
