package config

import "testing"

func TestCameraCredentialsPreserveAndRestore(t *testing.T) {
	old := defaults()
	old.Admin.Password = "currentAdmin"
	old.Stream.ReadPass = "currentRead"
	old.Cameras = []Camera{{ID: "cam1", PublishPass: "currentPublish"}}
	next := defaults()
	next.Cameras = []Camera{{PublishPass: "publishpass", ID: "cam1"}, {ID: "cam2"}}
	if err := next.PreserveCredentials(old, true, false); err != nil {
		t.Fatal(err)
	}
	if next.Cameras[0].PublishPass != old.Cameras[0].PublishPass || next.Cameras[1].PublishPass != "" {
		t.Fatal("preview changed credentials")
	}
	if err := next.PreserveCredentials(old, true, true); err != nil {
		t.Fatal(err)
	}
	if next.Admin.Password != old.Admin.Password || next.Stream.ReadPass != old.Stream.ReadPass {
		t.Fatal("restore lost current credentials")
	}
	p := next.Cameras[1].PublishPass
	if len(p) != 24 {
		t.Fatal("wrong generated length")
	}
	for _, c := range p {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			t.Fatal("wrong generated alphabet")
		}
	}
	if err := next.PreserveCredentials(nil, true, true); err != nil {
		t.Fatal(err)
	}
	if next.Admin.Password == old.Admin.Password || next.Cameras[0].PublishPass == old.Cameras[0].PublishPass {
		t.Fatal("unreadable current file reused credentials")
	}
}

func TestReadUserCannotImpersonateCamera(t *testing.T) {
	for _, id := range []string{"cam1", "cam2", "cam3", "cam4", "cam5", "cam6", "cam7", "cam8"} {
		c := valid()
		c.Stream.ReadUser = id
		if c.Validate() == nil {
			t.Fatal("camera id accepted as read user")
		}
	}
}
func TestLegacyPublishCredentialsRejected(t *testing.T) {
	for _, key := range []string{"publish_user", "publish_pass"} {
		if _, err := Parse([]byte("stream:\n  " + key + ": legacy\n")); err == nil {
			t.Fatal("legacy stream credential accepted")
		}
	}
}
