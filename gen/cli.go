package gen

import (
	"bytes"
	"embed"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/lestrrat-go/xstrings"
	"github.com/urfave/cli/v2"
	"golang.org/x/mod/modfile"
)

//go:embed tmpl/*
var embedded embed.FS

type App struct {
}

type genCtx struct {
	srcDir           string
	srcPkg           string
	srcModule        string
	srcModulePath    string
	srcModuleVersion string
	usrDirs          []string
	dstDir           string
	tmpDir           string
	devMode          bool
	devPath          string
	genHasMethods    bool
}

func (app *App) Run(args []string) error {
	cliapp := cli.App{
		// cmd <schema_dir> -tmpl-dir=<dir1> -dst-dir=<dir>
		Name:   "sketch",
		Usage:  "Generate code from schema",
		Action: app.RunMain,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "dev-mode",
				Usage: "enable developer mode (only for sketch devs)",
			},
			&cli.StringFlag{
				Name:  "dev-path",
				Usage: "path to the sketch source code (default: current dir)",
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
		},
	}

	return cliapp.Run(args)
}

type DeclaredSchema struct {
	Name string
}

var reMajorVersion = regexp.MustCompile(`v\d+$`)

func (app *App) RunMain(c *cli.Context) error {
	// Prepare the context
	if c.NArg() != 1 {
		cli.ShowAppHelp(c)
		return fmt.Errorf(`schema directory must be supplied`)
	}

	srcDir := c.Args().Get(0)
	// srcDir must be absolute
	absSrcDir, err := filepath.Abs(srcDir)
	if err != nil {
		return fmt.Errorf(`failed to get absolute path for %q: %w`, srcDir, err)
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
	defer os.RemoveAll(tmpDir)

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

	rel, err := filepath.Rel(tmpDir, moduleDir)
	if err != nil {
		return fmt.Errorf(`failed to find relative path for %q: %w`, moduleDir, err)
	}

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

	ctx := genCtx{
		srcDir:           srcDir,
		srcPkg:           filepath.Clean(filepath.Join(parsedMod.Module.Mod.Path, schemaDir)),
		srcModule:        srcModule,
		srcModulePath:    rel,
		srcModuleVersion: srcModuleVersion,
		dstDir:           dstDir,
		tmpDir:           tmpDir,
		usrDirs:          usrDirs,
		devMode:          c.Bool(`dev-mode`),
		devPath:          c.String(`dev-path`),
		genHasMethods:    c.Bool(`with-has-methods`),
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
		"tmpl/result.tmpl",
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

	schemaMap := make(map[string]string)
	for _, schema := range schemas {
		schemaMap[xstrings.Snake(schema.Name)+"_gen.go"] = schema.Name
	}
	if err := app.generateCompilerMain(ctx, tmpl, schemaMap); err != nil {
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

func (app *App) makeVars(ctx *genCtx) (map[string]interface{}, error) {
	vars := map[string]interface{}{
		"SrcPkg":             ctx.srcPkg,
		"SrcModule":          ctx.srcModule,
		"SrcModulePath":      ctx.srcModulePath,
		"SrcModuleVersion":   ctx.srcModuleVersion,
		"UserTemplateDirs":   ctx.usrDirs,
		"GenerateHasMethods": ctx.genHasMethods,
	}

	if ctx.devMode {
		devpath := ctx.devPath
		if devpath == "" {
			wd, err := os.Getwd()
			if err != nil {
				return nil, fmt.Errorf(`failed to compute working directory: %w`, err)
			}
			devpath = wd
		}
		rel, err := filepath.Rel(ctx.tmpDir, devpath)
		if err != nil {
			return nil, fmt.Errorf(`failed to compute relative path betwen %q and %q: %w`, ctx.tmpDir, devpath, err)
		}
		vars["DevPath"] = rel
	}

	return vars, nil
}

func (app *App) generateGoMod(ctx *genCtx, tmpl *template.Template) error {
	var buf bytes.Buffer

	vars, err := app.makeVars(ctx)
	if err != nil {
		return fmt.Errorf(`failed to build variable map: %w`, err)
	}
	if err := tmpl.ExecuteTemplate(&buf, "go.mod", vars); err != nil {
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

func (app *App) generateCompilerMain(ctx *genCtx, tmpl *template.Template, schemaMap map[string]string) error {
	dstpath := filepath.Join(ctx.tmpDir, `main.go`)
	f, err := os.OpenFile(dstpath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf(`failed to open file %q: %w`, dstpath, err)
	}
	defer f.Close()

	vars, err := app.makeVars(ctx)
	if err != nil {
		return fmt.Errorf(`failed to build variable map: %w`, err)
	}
	vars["Schemas"] = schemaMap
	if err := tmpl.ExecuteTemplate(f, "main.go", vars); err != nil {
		return fmt.Errorf(`failed to execute template "go.mod": %w`, err)
	}
	return nil
}

func (app *App) buildCompiler(ctx *genCtx) error {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = ctx.tmpDir
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf(`failed to run go mod tidy: %w`, err)
	}

	cmd = exec.Command("go", "build", "-o", "sketch")
	cmd.Dir = ctx.tmpDir
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf(`failed to run go build:%w`, err)
	}

	cmd = exec.Command("./sketch", ctx.dstDir)
	cmd.Dir = ctx.tmpDir
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf(`failed to run go build:%w`, err)
	}
	return nil
}
