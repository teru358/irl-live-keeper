package winproc

import "testing"

func TestWindowsCommandLineQuotesArguments(t *testing.T) {
	urlExe, urlArgs := URLCommand("http://127.0.0.1:8080/")
	if urlExe != "rundll32.exe" || WindowsCommandLine(urlArgs...) != `rundll32.exe url.dll,FileProtocolHandler http://127.0.0.1:8080/` {
		t.Fatalf("URL command = %q %q", urlExe, WindowsCommandLine(urlArgs...))
	}
	folderExe, folderArgs := FolderCommand(`C:\Users\Some User\logs`)
	if folderExe != "explorer.exe" || WindowsCommandLine(folderArgs...) != `explorer.exe "C:\Users\Some User\logs"` {
		t.Fatalf("folder command = %q %q", folderExe, WindowsCommandLine(folderArgs...))
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"rundll32.exe", "url.dll,FileProtocolHandler", "http://127.0.0.1:8080/a"}, `rundll32.exe url.dll,FileProtocolHandler http://127.0.0.1:8080/a`},
		{[]string{"explorer.exe", `C:\Users\Some User\logs`}, `explorer.exe "C:\Users\Some User\logs"`},
		{[]string{"tool.exe", `C:\folder with space\`}, `tool.exe "C:\folder with space\\"`},
	} {
		if got := WindowsCommandLine(tc.args...); got != tc.want {
			t.Errorf("WindowsCommandLine(%q) = %q, want %q", tc.args, got, tc.want)
		}
	}
}
