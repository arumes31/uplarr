package main

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

func main() {
	paths := []string{"/upload/file.txt", "/etc/passwd", "/upload/../etc/passwd", "../forbidden", "/", "/root"}
	base := "/"
	
	for _, p := range paths {
		cleanedP := path.Clean(filepath.ToSlash(p))
		
		effectiveBase := base
		if !path.IsAbs(cleanedP) {
			effectiveBase = "/upload" // mock RemoteDir
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
