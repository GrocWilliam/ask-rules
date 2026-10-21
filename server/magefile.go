//go:build mage

// magefile.go — Tâches de développement du serveur (`mage -l` pour la liste)
package main

import (
	"os"
	"os/exec"
)

// Default est lancé par `mage` sans argument.
var Default = Test

// Dev lance le serveur avec rechargement à chaud (air).
func Dev() error {
	return run("air")
}

// Build compile le serveur dans ./ask-rules-server.
func Build() error {
	return run("go", "build", "-o", "ask-rules-server", "./cmd/server")
}

// Test lance go vet puis les tests.
func Test() error {
	if err := run("go", "vet", "./..."); err != nil {
		return err
	}
	return run("go", "test", "./...")
}

// Eval mesure la qualité du retrieval (DATABASE_URL doit pointer vers une base dédiée).
func Eval() error {
	return run("go", "run", "./cmd/eval", "-index", "-v")
}

// run exécute une commande en affichant sa sortie dans le terminal.
func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}
