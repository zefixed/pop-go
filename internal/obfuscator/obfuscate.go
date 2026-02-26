package obfuscator

func (o *Obfuscator) Obfuscate() error {
	if o.cfg.Obfuscator.Comments {
		o.DeleteComments()
	}

	err := o.literals.Obfuscate(o.cfg.Obfuscator.Literals.Level)
	if err != nil {
		return err
	}

	return nil
}
