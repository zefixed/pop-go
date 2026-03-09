package obfuscator

func (o *Obfuscator) Obfuscate() error {
	// Deleting comments if flag is set
	if o.cfg.Obfuscator.Comments.Enable {
		o.DeleteComments()
	}

	// Applying control flow flattening
	if o.cfg.Obfuscator.ControlFlow.Enable {
		o.controlFlow.Obfuscate()
	}

	// Renaming non-exported identifiers if flag is set
	if o.cfg.Obfuscator.Identifiers.Enable {
		err := o.renameIdentifiers.Obfuscate()
		if err != nil {
			return err
		}
	}

	// Obfuscating literals if flag is set by level from config
	if o.cfg.Obfuscator.Literals.Enable {
		err := o.literals.Obfuscate(o.cfg.Obfuscator.Literals.Level)
		if err != nil {
			return err
		}
	}

	// Writing changes to files
	err := o.WriteAll()
	if err != nil {
		return err
	}

	return nil
}
