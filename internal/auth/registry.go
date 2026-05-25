package auth

// All returns the supported auth methods in the order shown in the UI.
func All() []Method {
	return []Method{
		&Userpass{},
		&Token{},
		&AppRole{},
	}
}

// ByName returns the Method matching the given name, or nil if not found.
func ByName(name string) Method {
	for _, m := range All() {
		if m.Name() == name {
			return m
		}
	}
	return nil
}
