package gen

import (
	"bufio"
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"github.com/urfave/cli/v2"
	"golang.org/x/mod/modfile"
)

//go:embed tmpl/*
var embedded embed.FS

type App struct {
	verbose bool
}

func (app *App) Infof(f string, args ...interface{}) {
	if !app.verbose {
		return
	}
	if !strings.HasPrefix(f, "\n") {
		f += "\n"
	}
	fmt.Fprintf(os.Stdout, f, args...)
}

func (app *App) DumpJSON(v interface{}) {
	txt, err := json.MarshalIndent(v, "", "   ")
	if err != nil {
		return
	}
	scanner := bufio.NewScanner(bytes.NewReader(txt))
	for scanner.Scan() {
		app.Infof("     | %s", scanner.Text())
	}
}

type genCtx struct {
	srcDir    string
	usrDirs   []string
	dstDir    string
	tmpDir    string
	variables map[string]interface{}
}

func (app *App) Run(args []string) error {
	cliapp := cli.App{
		// cmd <schema_dir> -tmpl-dir=<dir1> -dst-dir=<dir>
		Name:   "sketch",
		Usage:  "Generate code from schema",
		Action: app.RunMain,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "verbose",
				Usage: "Output verbose logging to stdout",
			},
			&cli.BoolFlag{
				Name:  "remove-tmpdir",
				Usage: "Set to false to inspect intermediate artifacts (default: false)",
				Value: true,
			},
			&cli.BoolFlag{
				Name:  "dev-mode",
				Usage: "enable developer mode (only for sketch devs)",
			},
			&cli.StringSliceFlag{
				Name:  "var",
				Usage: "A key=value pair of variables, followed by an optional type (e.g. key=value:bool)",
			},
			&cli.StringFlag{
				Name:  "dev-path",
				Usage: "path to the sketch source code (default: current dir)",
			},
			&cli.BoolFlag{
				Name:  "with-builders",
				Usage: "enable generating Builders for each object",
			},
			&cli.BoolFlag{
				Name:  "with-key-name-prefix",
				Usage: "prepend object names in key name constant variables",
			},
			&cli.BoolFlag{
				Name:  "with-has-methods",
				Usage: "enable generating HasXXXX methods for each attribute",
			},
			&cli.StringFlag{
				Name:    `dst-dir`,
				Aliases: []string{"d"},
				Usage:   "use `DIR` as destination to write generated files (default: current directory)",
			},
			&cli.StringSliceFlag{
				Name:    "tmpl-dir",
				Aliases: []string{"t"},
				Usage:   "user-supplied extra templates",
			},
			&cli.StringSliceFlag{
				Name:  "exclude-method",
				Usage: "Regular expression to match against method names. If they match the method will not be generated. If schemas define their own GenerateMethod, these patterns will be ignored",
			},
		},
	}

	return cliapp.Run(args)
}

type DeclaredSchema struct {
	Name string
}

var reMajorVersion = regexp.MustCompile(`v\d+$`)
var reMatchVar = regexp.MustCompile(`([^=]+)=(.+)(?::(bool|string|int))?`)

func (app *App) RunMain(c *cli.Context) error {
	// Prepare the context
	if c.NArg() != 1 {
		cli.ShowAppHelp(c)
		return fmt.Errorf(`schema directory must be supplied`)
	}

	app.verbose = c.Bool(`verbose`)

	variables := make(map[string]interface{})
	if vars := c.StringSlice(`var`); len(vars) > 0 {
		for _, sv := range vars {
			matches := reMatchVar.FindAllStringSubmatch(sv, -1)
			if len(matches) == 0 {
				return fmt.Errorf(`invalid variable declaration %q`, sv)
			}

			name := matches[0][1]
			typ := matches[0][3]

			switch typ {
			case "", "string":
				variables[name] = matches[0][2]
			case "int":
				i, err := strconv.ParseInt(matches[0][2], 10, 64)
				if err != nil {
					return fmt.Errorf(`failed to parse %q as bool: %w`, name, err)
				}
				variables[name] = i
			case "bool":
				b, err := strconv.ParseBool(matches[0][2])
				if err != nil {
					return fmt.Errorf(`failed to parse %q as bool: %w`, name, err)
				}
				variables[name] = b
			default:
				return fmt.Errorf(`unhandled variable type %q for %q`, typ, name)
			}
		}
	}
	variables["Verbose"] = app.verbose

	if patterns := c.StringSlice(`exclude-method`); len(patterns) > 0 {
		variables["ExcludeMethods"] = patterns
	}

	srcDir := c.Args().Get(0)

	app.Infof(`👉 Accepted src directory %q`, srcDir)
	// srcDir must be absolute
	absSrcDir, err := filepath.Abs(srcDir)
	if err != nil {
		return fmt.Errorf(`failed to get absolute path for %q: %w`, srcDir, err)
	}
	if srcDir != absSrcDir {
		app.Infof(`   ✅ Converted src directory to %q`, absSrcDir)
	}
	srcDir = absSrcDir

	dstDir := c.String(`dst-dir`)
	if dstDir == "" {
		panic("WHY WHY WHY")
		dir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf(`failed to compute current working directory: %w`, err)
		}
		dstDir = dir
	}
	// dstDir must be absolute
	absDstDir, err := filepath.Abs(dstDir)
	if err != nil {
		return fmt.Errorf(`failed to get absolute path for %q: %w`, dstDir, err)
	}
	dstDir = absDstDir

	tmpDir, err := os.MkdirTemp("", "sketch-*")
	if err != nil {
		return fmt.Errorf(`failed to create temporary directory: %w`, err)
	}
	defer func() {
		if !c.Bool(`remove-tmpdir`) {
			app.Infof(`👉 NOT removing temporary working directory %q`, tmpDir)
			return
		}

		app.Infof(`👉 Removing temporary working directory %q`, tmpDir)
		os.RemoveAll(tmpDir)
	}()
	app.Infof(`👉 Created temporary working directory %q`, tmpDir)

	var moduleDir string
	var gomodFn string
	for srcDir := srcDir; len(srcDir) > 0; {
		gomodFn = filepath.Join(srcDir, `go.mod`)
		if _, err := os.Stat(gomodFn); err == nil {
			moduleDir = srcDir
			break
		}
		srcDirComps := strings.Split(srcDir, sepStr)
		srcDirComps = srcDirComps[:len(srcDirComps)-1]
		srcDir = strings.Join(srcDirComps, sepStr)
	}

	if moduleDir == "" {
		return fmt.Errorf(`failed to find go.mod`)
	}

	app.Infof(`👉 Accepted module directory %q`, moduleDir)

	gomodContent, err := os.ReadFile(gomodFn)
	if err != nil {
		return fmt.Errorf(`failed to read from %q: %w`, gomodFn, err)
	}

	parsedMod, err := modfile.Parse(gomodFn, gomodContent, nil)
	if err != nil {
		return fmt.Errorf(`failed to parse %q: %w`, gomodFn, err)
	}

	schemaDir, err := filepath.Rel(moduleDir, srcDir)
	if err != nil {
		return fmt.Errorf(`failed to get relative path from %q to %q: %w`, moduleDir, srcDir, err)
	}

	var usrDirs []string
	for _, usrDir := range c.StringSlice(`tmpl-dir`) {
		abs, err := filepath.Abs(usrDir)
		if err != nil {
			return fmt.Errorf(`failed to get absolute path for %q: %w`, usrDir, err)
		}
		usrDirs = append(usrDirs, abs)
	}

	srcModule := parsedMod.Module.Mod.Path
	srcModuleVersion := "v0.0.0"
	if majorV := reMajorVersion.FindString(srcModule); majorV != "" {
		srcModuleVersion = majorV + ".0.0"
	}

	variables[`GenerateBuilders`] = c.Bool(`with-builders`)
	variables[`GenerateHasMethods`] = c.Bool(`with-has-methods`)
	variables[`SrcModule`] = srcModule
	variables[`SrcModulePath`] = moduleDir
	variables[`SrcModuleVersion`] = srcModuleVersion
	variables[`SrcPkg`] = filepath.Clean(filepath.Join(parsedMod.Module.Mod.Path, schemaDir))
	variables[`UserTemplateDirs`] = usrDirs
	variables[`WithKeyNamePrefix`] = c.Bool(`with-key-name-prefix`)
	if c.Bool(`dev-mode`) {
		devpath := c.String(`dev-path`)
		if devpath == "" {
			wd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf(`failed to compute working directory: %w`, err)
			}
			devpath = wd
		}
		variables[`DevPath`] = devpath
	}

	ctx := genCtx{
		srcDir:    srcDir,
		dstDir:    dstDir,
		tmpDir:    tmpDir,
		usrDirs:   usrDirs,
		variables: variables,
	}

	schemas, err := app.extractStructs(&ctx)
	if err != nil {
		return err
	}

	// Using these schemas, we dynamically generate some source code
	// that can generate the code for the client
	if err := app.genCompiler(&ctx, schemas); err != nil {
		return err
	}

	if err := app.buildCompiler(&ctx); err != nil {
		return fmt.Errorf(`failed to build compiler: %w`, err)
	}

	return nil
}

func (app *App) extractStructs(ctx *genCtx) ([]*DeclaredSchema, error) {
	dir := ctx.srcDir
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, 0)
	if err != nil {
		return nil, err
	}

	var schemas []*DeclaredSchema
	for _, pkg := range pkgs {
		// There should be only one package
		for _, file := range pkg.Files {
			schemaPkg := "schema"
			for _, imp := range file.Imports {
				if imp.Path.Value == `"github.com/lestrrat-go/sketch/schema"` {
					if imp.Name != nil {
						schemaPkg = imp.Name.Name
					}
				}
			}

			for _, node := range file.Decls {
				switch node := node.(type) {
				case *ast.GenDecl:
					for _, spec := range node.Specs {
						switch spec := spec.(type) {
						case *ast.TypeSpec:
							structName := spec.Name.Name
							switch specType := spec.Type.(type) {
							case *ast.StructType:
								if app.looksLikeSchema(schemaPkg, specType) {
									schemas = append(schemas, &DeclaredSchema{
										Name: structName,
									})
								}
							}
						}
					}
				}
			}
		}
	}
	return schemas, nil
}

func (app *App) looksLikeSchema(schemaPkg string, specType *ast.StructType) bool {
	for _, field := range specType.Fields.List {
		// The name should be empty
		if len(field.Names) != 0 {
			continue
		}

		ident, ok := field.Type.(*ast.SelectorExpr)
		if !ok {
			continue
		}

		// ident.X should be the schema name
		pkgIdent := ident.X.(*ast.Ident)
		if pkgIdent.Name != schemaPkg {
			continue
		}

		if ident.Sel.Name != "Base" {
			continue
		}

		return true
	}
	return false
}

func (app *App) genCompiler(ctx *genCtx, schemas []*DeclaredSchema) error {
	// Copy files
	toCopy := []string{
		"tmpl/builder.tmpl",
		"tmpl/object.tmpl",
	}
	for _, name := range toCopy {
		to := filepath.Join(ctx.tmpDir, name)
		dir := filepath.Dir(to)
		if _, err := os.Stat(dir); err != nil {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf(`failed to createdirectory %q: %w`, dir, err)
			}
		}

		src, err := embedded.Open(name)
		if err != nil {
			return fmt.Errorf(`failed to open file %q for reading: %w`, name, err)
		}

		dst, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			src.Close()
			return fmt.Errorf(`failed to open file %q for writing: %w`, to, err)
		}
		io.Copy(dst, src)

		src.Close()
		dst.Close()
	}

	tmpl, err := template.ParseFS(embedded, "tmpl/compiler.tmpl")
	if err != nil {
		return fmt.Errorf(`failed to compile template: %w`, err)
	}

	if err := app.generateGoMod(ctx, tmpl); err != nil {
		return fmt.Errorf(`failed to generate go.mod: %w`, err)
	}

	if err := app.generateCompilerMain(ctx, tmpl, schemas); err != nil {
		return fmt.Errorf(`failed to generate compiler code: %w`, err)
	}
	return nil
}

var sepStr string

func init() {
	var sb strings.Builder
	sb.WriteRune(filepath.Separator)
	sepStr = sb.String()
}

func (app *App) generateGoMod(ctx *genCtx, tmpl *template.Template) error {
	app.Infof(`👉 Generating go.mod`)
	app.Infof(`  👉 Rendering template with following variables:`)
	app.DumpJSON(ctx.variables)

	var buf bytes.Buffer

	if err := tmpl.ExecuteTemplate(&buf, "compiler/go.mod", ctx.variables); err != nil {
		return fmt.Errorf(`failed to execute template "go.mod": %w`, err)
	}

	dstpath := filepath.Join(ctx.tmpDir, `go.mod`)
	f, err := os.OpenFile(dstpath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf(`failed to open file %q: %w`, dstpath, err)
	}
	defer f.Close()

	buf.WriteTo(f)
	return nil
}

func (app *App) generateCompilerMain(ctx *genCtx, tmpl *template.Template, schemas []*DeclaredSchema) error {
	app.Infof(`👉 Generating main.go`)
	dstpath := filepath.Join(ctx.tmpDir, `main.go`)
	f, err := os.OpenFile(dstpath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf(`failed to open file %q: %w`, dstpath, err)
	}
	defer f.Close()

	localVars := make(map[string]interface{})
	for k, v := range ctx.variables {
		localVars[k] = v
	}
	localVars["Schemas"] = schemas
	app.Infof(`  👉 Rendering template with following variables:`)
	app.DumpJSON(localVars)
	if err := tmpl.ExecuteTemplate(f, "compiler/main.go", localVars); err != nil {
		return fmt.Errorf(`failed to execute template "go.mod": %w`, err)
	}
	return nil
}

func (app *App) buildCompiler(ctx *genCtx) error {
	dumpMain := func() {
		f, err := os.Open(filepath.Join(ctx.tmpDir, "main.go"))
		if err == nil {
			defer f.Close()

			scanner := bufio.NewScanner(f)
			i := 1
			for scanner.Scan() {
				fmt.Fprintf(os.Stderr, "%04d: %s\n", i, scanner.Text())
				i++
			}
		}
	}

	app.Infof(`👉 Running "go mod tidy"`)
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = ctx.tmpDir
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		dumpMain()
		return fmt.Errorf(`failed to run go mod tidy: %w`, err)
	}

	app.Infof(`👉 Running "go build -o sketch-compiler"`)
	cmd = exec.Command("go", "build", "-o", "sketch-compiler")
	cmd.Dir = ctx.tmpDir
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		dumpMain()
		return fmt.Errorf(`failed to run go build: %w`, err)
	}

	app.Infof(`👉 Running "./sketch-compiler"`)
	cmd = exec.Command("./sketch-compiler", ctx.dstDir)
	cmd.Dir = ctx.tmpDir
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf(`failed to run go build:%w`, err)
	}
	return nil
}
