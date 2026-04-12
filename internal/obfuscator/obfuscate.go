package obfuscator

// Obfuscate runs all enabled obfuscation passes in the correct order and
// writes the transformed AST back to the temporary directory.
//
// Pass order matters:
//  1. DeleteComments — must run first so subsequent passes see clean ASTs.
//  2. RenameIdentifiers — must run before CFF because CFF creates synthetic
//     AST nodes (hoisted var declarations) that are invisible to TypesInfo;
//     renaming after CFF would leave those nodes with stale names.
//  3. ControlFlow — runs after renaming so all hoisted names are already
//     consistent with their usages.
//  4. Literals — encrypts string constants after structural changes are final.
//  5. WriteAll — serialises every modified AST file to disk.
func (o *Obfuscator) Obfuscate() error {
	if o.cfg.Obfuscator.Comments.Enable {
		o.DeleteComments()
	}

	if o.cfg.Obfuscator.Identifiers.Enable {
		err := o.renameIdentifiers.Obfuscate()
		if err != nil {
			return err
		}
	}

	if o.cfg.Obfuscator.ControlFlow.Enable {
		o.controlFlow.Obfuscate()
	}

	if o.cfg.Obfuscator.Literals.Enable {
		err := o.literals.Obfuscate(o.cfg.Obfuscator.Literals.Level)
		if err != nil {
			return err
		}
	}

	return o.WriteAll()
}
