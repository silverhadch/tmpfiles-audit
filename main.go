package main

import (
"bufio"
"fmt"
"os"
"path/filepath"
"regexp"
"strings"
)

var (
lineRegex = regexp.MustCompile(`^L\s+([^\s]+)\s+[^\s]*\s+[^\s]*\s+[^\s]*\s+(.*)$`)

colorReset   = "\033[0m"
colorGreen   = "\033[32m"
colorYellow  = "\033[33m"
colorRed     = "\033[31m"
colorBoldRed = "\033[1;31m"
)

func cleanQuotes(s string) string {
s = strings.TrimSpace(s)
s = strings.Trim(s, `"`)
s = strings.Trim(s, `'`)
return s
}

func factoryTarget(path string) string {
filename := filepath.Base(path)
if filename == "etc" || strings.HasPrefix(path, "/etc/") {
return "/usr/share/factory/etc/" + strings.TrimPrefix(path, "/etc/")
} else if filename == "var" || strings.HasPrefix(path, "/var/") {
return "/usr/share/factory/var/" + strings.TrimPrefix(path, "/var/")
}
return "/usr/share/factory" + path
}

func processLine(line string, linkedDirs map[string]map[string]bool) error {
matches := lineRegex.FindStringSubmatch(line)
if matches == nil {
return nil
}

path := matches[1]
target := cleanQuotes(matches[2])

if target == "" || target == "-" {
ft := factoryTarget(path)
fmt.Printf("L %s -> (factory default: %s)\n", path, ft)
if _, err := os.Stat(ft); err == nil {
fmt.Printf("  %s✓ Factory target exists: %s%s\n", colorGreen, ft, colorReset)
} else {
fmt.Printf("  %s✗ Factory target missing: %s%s\n", colorRed, ft, colorReset)
return fmt.Errorf("missing factory target: %s", ft)
}
} else {
fmt.Printf("L %s -> %s\n", path, target)
if _, err := os.Stat(target); err == nil {
fmt.Printf("  %s✓ Target exists: %s%s\n", colorGreen, target, colorReset)
dir := filepath.Dir(target)
if !isBaseDir(dir) {
if _, ok := linkedDirs[dir]; !ok {
linkedDirs[dir] = make(map[string]bool)
}
linkedDirs[dir][filepath.Base(target)] = true
}
} else {
fmt.Printf("  %s✗ Target missing: %s%s\n", colorRed, target, colorReset)
return fmt.Errorf("missing target: %s", target)
}
}
return nil
}

func isBaseDir(dir string) bool {
baseDirs := []string{"/etc", "/var", "/usr", "/bin", "/sbin", "/lib", "/lib64"}
for _, b := range baseDirs {
if dir == b {
return true
}
}
return false
}

func loadIgnoreFiles() map[string]bool {
ignoredFiles := make(map[string]bool)
files, err := filepath.Glob("/usr/share/tmpfiles.d/*.ignore")
if err != nil {
return ignoredFiles
}

for _, file := range files {
f, err := os.Open(file)
if err != nil {
continue
}
scanner := bufio.NewScanner(f)
for scanner.Scan() {
line := strings.TrimSpace(scanner.Text())
if line == "" || strings.HasPrefix(line, "#") {
continue
}
ignoredFiles[line] = true
fmt.Printf("   %s⤷ Ignore rule: skip %s (from %s)%s\n", colorYellow, line, file, colorReset)
}
f.Close()
}
return ignoredFiles
}

func checkDirectoryCompleteness(linkedDirs map[string]map[string]bool, ignoredFiles map[string]bool) error {
hadError := false
for dir, linkedFiles := range linkedDirs {
entries, err := os.ReadDir(dir)
if err != nil {
continue
}

missing := []string{}
for _, entry := range entries {
if entry.IsDir() {
continue
}
fullPath := filepath.Join(dir, entry.Name())
if ignoredFiles[fullPath] {
continue
}
if !linkedFiles[entry.Name()] {
missing = append(missing, entry.Name())
}
}
if len(missing) > 0 {
fmt.Printf("%s✗ Error: Directory %s has symlinks in tmpfiles.d but not all files are linked.%s\n", colorRed, dir, colorReset)
fmt.Printf("   Missing files: %s%s%s\n", colorRed, strings.Join(missing, ", "), colorReset)
hadError = true
}
}
if hadError {
return fmt.Errorf("incomplete directory linking detected")
}
return nil
}

func printSummary(linkedDirs map[string]map[string]bool, ignoredFiles map[string]bool) {
fmt.Println("\n=== Summary of Linked/Ignored/Missing Files ===")
for dir, linkedFiles := range linkedDirs {
entries, err := os.ReadDir(dir)
if err != nil {
fmt.Printf("%sDirectory: %s (cannot read: %v)%s\n", colorRed, dir, err, colorReset)
continue
}

missing := []string{}
ignored := []string{}
actualLinked := []string{}

for _, entry := range entries {
if entry.IsDir() {
continue
}
fullPath := filepath.Join(dir, entry.Name())
if ignoredFiles[fullPath] {
ignored = append(ignored, entry.Name())
} else if linkedFiles[entry.Name()] {
actualLinked = append(actualLinked, entry.Name())
} else {
missing = append(missing, entry.Name())
}
}

if len(missing) > 0 {
fmt.Printf("\n%sDirectory: %s%s\n", colorBoldRed, dir, colorReset)
} else {
fmt.Printf("\nDirectory: %s\n", dir)
}

if len(actualLinked) > 0 {
fmt.Printf("  Linked files: %s%s%s\n", colorGreen, strings.Join(actualLinked, ", "), colorReset)
}
if len(ignored) > 0 {
fmt.Printf("  Ignored files: %s%s%s\n", colorYellow, strings.Join(ignored, ", "), colorReset)
}
if len(missing) > 0 {
fmt.Printf("  Missing files: %s%s%s\n", colorRed, strings.Join(missing, ", "), colorReset)
} else {
fmt.Println("  All files properly linked or ignored.")
}
}
}

func main() {
files, err := filepath.Glob("/usr/lib/tmpfiles.d/*.conf")
if err != nil {
fmt.Fprintf(os.Stderr, "Error finding files: %v\n", err)
os.Exit(1)
}

exitCode := 0
linkedDirs := make(map[string]map[string]bool)

for _, file := range files {
f, err := os.Open(file)
if err != nil {
fmt.Fprintf(os.Stderr, "Error opening file %s: %v\n", file, err)
exitCode = 1
continue
}
scanner := bufio.NewScanner(f)
for scanner.Scan() {
line := scanner.Text()
if !strings.HasPrefix(line, "L") || strings.HasPrefix(line, "L?") || strings.HasPrefix(line, "L+") {
continue
}
if err := processLine(line, linkedDirs); err != nil {
exitCode = 1
}
}
f.Close()
}

ignoredFiles := loadIgnoreFiles()
if err := checkDirectoryCompleteness(linkedDirs, ignoredFiles); err != nil {
exitCode = 1
}

printSummary(linkedDirs, ignoredFiles)

os.Exit(exitCode)
}
