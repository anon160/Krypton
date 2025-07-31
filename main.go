package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func indent(level int) string {
	return strings.Repeat("    ", level)
}

// Track variable assignment mappings
var varExprs = make(map[string]string)

func isAssignment(line string) (name string, value string, ok bool) {
	if strings.Contains(line, " be ") {
		parts := strings.Split(line, " be ")
		if len(parts) == 2 {
			return parts[0], parts[1], true
		}
	}
	return "", "", false
}

func convertFString(content string) (string, []string) {
	vars := []string{}
	// Replace {var} with {}, and collect var names
	formatted := regexp.MustCompile(`{([^}]+)}`).ReplaceAllStringFunc(content, func(m string) string {
		varName := m[1 : len(m)-1]
		vars = append(vars, varName)
		return "{}"
	})
	return formatted, vars
}

func handleFString(line string) string {
	// Handle: x = f"{n}"
	assignRe := regexp.MustCompile(`(\w+)\s*=\s*f\"{(\w+)}\"`)
	if matches := assignRe.FindStringSubmatch(line); len(matches) == 3 {
		varName, expr := matches[1], matches[2]
		varExprs[varName] = expr
		return fmt.Sprintf("let %s = format!(\"{}\", %s);", varName, expr)
	}

	// Handle: println!(f"Hello {n}")
	fStrRe := regexp.MustCompile(`f\"([^\"]*)\"`)
	matches := fStrRe.FindStringSubmatch(line)
	if len(matches) < 2 {
		return line
	}

	content := matches[1]
	formatted, vars := convertFString(content)
	formatCall := fmt.Sprintf("format!(\"%s\"%s)", formatted, ifNotEmpty(strings.Join(vars, ", ")))

	if strings.HasPrefix(line, "println!(") {
		return fmt.Sprintf("println!(\"{}\", %s);", formatCall)
	}

	// General fallback: replace f-string with format!(...)
	return strings.Replace(line, matches[0], formatCall, 1)
}

func ifNotEmpty(s string) string {
	if s == "" {
		return ""
	}
	return ", " + s
}

func translateLine(line string) string {
	line = strings.TrimSpace(line)

	// Handle: println!(x) → println!("{}", x);
	printVarRe := regexp.MustCompile(`^println!\((\w+)\)$`)
	if matches := printVarRe.FindStringSubmatch(line); len(matches) == 2 {
		varName := matches[1]
		return fmt.Sprintf("println!(\"{}\", %s);", varName)
	}

	if strings.Contains(line, `f"`) || strings.HasPrefix(line, "print(f") {
		line = handleFString(line)
	}

	if name, value, ok := isAssignment(line); ok {
		return fmt.Sprintf("let %s = %s;", name, value)
	}

	if strings.HasSuffix(line, "()") {
		return line + ";"
	}

	return line
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <source-file.lang>")
		return
	}

	inputFile := os.Args[1]
	file, err := os.Open(inputFile)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	outputFile := strings.TrimSuffix(filepath.Base(inputFile), filepath.Ext(inputFile)) + ".rs"
	output, err := os.Create(outputFile)
	if err != nil {
		fmt.Println("Error creating output file:", err)
		return
	}
	defer output.Close()

	scanner := bufio.NewScanner(file)
	var rustCode []string
	inFunction := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasSuffix(trimmed, "{") && strings.HasSuffix(strings.Split(trimmed, " ")[0], "()"):
			funcName := strings.Split(trimmed, "(")[0]
			rustCode = append(rustCode, fmt.Sprintf("fn %s() {", funcName))
			inFunction = true

		case inFunction && trimmed == "}":
			rustCode = append(rustCode, "}")
			inFunction = false

		case inFunction:
			rustCode = append(rustCode, indent(1)+translateLine(trimmed))

		default:
			rustCode = append(rustCode, translateLine(trimmed))
		}
	}

	for _, line := range rustCode {
		fmt.Fprintln(output, line)
	}

	fmt.Printf("✅ Transpilation complete. Output written to: %s\n", outputFile)
}
