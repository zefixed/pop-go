package obfuscator

func (o *Obfuscator) Obfuscate() error {
	// Deleting comments if flag is set
	if o.cfg.Obfuscator.Comments {
		o.DeleteComments()
	}

	// Obfuscating literals by level from config
	err := o.literals.Obfuscate(o.cfg.Obfuscator.Literals.Level)
	if err != nil {
		return err
	}

	// Renaming non-exported identifiers
	err = o.renameIdentifiers.Obfuscate()
	if err != nil {
		return err
	}

	// Writing changes to files
	err = o.WriteAll()
	if err != nil {
		return err
	}

	return nil
}
