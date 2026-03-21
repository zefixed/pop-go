package obfuscator

func (o *Obfuscator) Obfuscate() error {
	// Deleting comments if flag is set
	if o.cfg.Obfuscator.Comments.Enable {
		o.DeleteComments()
	}

	// Renaming non-exported identifiers BEFORE control flow flattening.
	// CFF creates new AST nodes (hoisted var declarations) that are invisible
	// to typesInfo, so renameIdentifiers must run first — while the AST still
	// matches typesInfo exactly.
	if o.cfg.Obfuscator.Identifiers.Enable {
		err := o.renameIdentifiers.Obfuscate()
		if err != nil {
			return err
		}
	}

	// Applying control flow flattening AFTER renaming.
	// At this point all identifiers are already renamed consistently,
	// so hoisted var names will match their usages.
	if o.cfg.Obfuscator.ControlFlow.Enable {
		o.controlFlow.Obfuscate()
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
