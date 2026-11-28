package config

// PreserveCredentials keeps revoked secrets out of saves and restores.
// A nil current config means recovery must generate every credential.
func (next *Config) PreserveCredentials(current *Config, restore, generate bool) error {
	if restore {
		if current != nil {
			next.Admin.Password = current.Admin.Password
			next.Stream.ReadUser, next.Stream.ReadPass = current.Stream.ReadUser, current.Stream.ReadPass
		} else if generate {
			var err error
			next.Admin.Password, err = GeneratePassword()
			if err != nil {
				return err
			}
			next.Stream.ReadUser = "obs"
			next.Stream.ReadPass, err = GeneratePassword()
			if err != nil {
				return err
			}
		}
	}
	for i := range next.Cameras {
		cam := &next.Cameras[i]
		cam.PublishPass = ""
		if current != nil {
			for _, old := range current.Cameras {
				if old.ID == cam.ID {
					cam.PublishPass = old.PublishPass
					break
				}
			}
		}
		if cam.PublishPass == "" && generate {
			var err error
			cam.PublishPass, err = GeneratePassword()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Config) PublishPassword(id string) string {
	for _, cam := range c.Cameras {
		if cam.ID == id {
			return cam.PublishPass
		}
	}
	return ""
}
func (c *Config) Secrets() []string {
	out := []string{c.Admin.Password, c.Stream.ReadPass, c.OBS.Password}
	for _, cam := range c.Cameras {
		out = append(out, cam.PublishPass)
	}
	return out
}
