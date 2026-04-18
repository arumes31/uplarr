package main

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

func main() {
	paths := []string{"/upload/file.txt", "/etc/passwd", "/upload/../etc/passwd", "../forbidden", "/", "/root"}
	
	for _, p := range paths {
		cleanedP := path.Clean(filepath.ToSlash(p))
		
		// Always validate against the intended boundary (RemoteDir)
		effectiveBase := "/upload" // mock RemoteDir
		if !path.IsAbs(cleanedP) {
			// Join relative path with base so filepath.Rel can detect traversal
			cleanedP = path.Join(effectiveBase, cleanedP)
		}
		
		rel, err := filepath.Rel(effectiveBase, cleanedP)
		if err != nil {
			fmt.Printf("Path: %s -> Error: %v\n", p, err)
			continue
		}
		
		posixRel := filepath.ToSlash(rel)
		traversal := posixRel == ".." || strings.HasPrefix(posixRel, "../")
		
		fmt.Printf("Path: %s -> Cleaned: %s, Base: %s, Rel: %s, PosixRel: %s, Traversal: %v\n", 
			p, cleanedP, effectiveBase, rel, posixRel, traversal)
	}
}
