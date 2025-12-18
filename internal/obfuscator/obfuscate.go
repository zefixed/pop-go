package obfuscator

func (o *Obfuscator) Obfuscate() {
	if o.cfg.Obfuscator.Comments {
		o.DeleteComments()
	}

	switch o.cfg.Obfuscator.Literals.Level {
	case "none":
		return
	case "easy":
		o.ObfuscateLiteralsEasy()
	}
}
