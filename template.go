package web

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aluka-7/utils"
)

const DefaultTplSetName = "DEFAULT"

var (
	TemplateEnv = "development"
	// 提供一个临时缓冲区以执行模板并捕获错误.
	bufPool = sync.Pool{
		New: func() interface{} { return new(bytes.Buffer) },
	}

	// 呈现HTML时包含的辅助功能
	helperFuncs = template.FuncMap{
		"yield":   func() (string, error) { return "", fmt.Errorf("没有定义布局就调用yield") },
		"current": func() (string, error) { return "", nil },
	}
)

// RenderOptions 表示用于指定Render中间件的配置选项的结构。.
type RenderOptions struct {
	Directory          string             // 加载模板目标.默认为"templates".
	AppendDirectories  []string           // 附加目录会覆盖默认模板.
	Layout             string             // 布局模板名称. 如果为""代表不会渲染布局.默认是"".
	Extensions         []string           // 用于从中解析模板文件的扩展. 默认值为[".tmpl", ".html"].
	Funcs              []template.FuncMap // Funcs是FuncMap的一部分,可在编译时应用于模板. 这对于助手功能很有用. 默认是[].
	Delims             Delims             // 将定界符设置为Delims结构中的指定字符串.
	Charset            string             // 将给定的字符集附加到Content-Type标头.默认是"UTF-8".
	HTMLContentType    string             // 允许将输出更改为XHTML而不是HTML.默认是"text/html".
	TemplateFileSystem                    // TemplateFileSystem是用于支持任何模板文件系统实现的接口.
}

// 代表用于HTML模板渲染的一组左右定界符
type Delims struct {
	Left  string // 左定界符，默认为{{
	Right string // 右定界符，默认为}}
}

// 表示能够列出所有文件的模板文件系统的接口.
type TemplateFileSystem interface {
	ListFiles() []TemplateFile
	Get(string) (io.Reader, error)
}

// 表示具有名称且可以读取的模板文件的接口.
type TemplateFile interface {
	Name() string
	Data() []byte
	Ext() string
}

// HTMLOptions是用于覆盖特定HTML调用的某些呈现选项的结构
type HTMLOptions struct {
	Layout string // 布局模板名称.覆盖Options.Layout.
}

// 初始化一个新的空模板集.
func NewTemplateSet() *TemplateSet {
	return &TemplateSet{
		sets: make(map[string]*template.Template),
		dirs: make(map[string]string),
	}
}

// 表示类型为*template.Template的模板集。
type TemplateSet struct {
	lock sync.RWMutex
	sets map[string]*template.Template
	dirs map[string]string
}

func (ts *TemplateSet) Set(name string, opt *RenderOptions) *template.Template {
	t := compile(*opt)
	ts.lock.Lock()
	defer ts.lock.Unlock()
	ts.sets[name] = t
	ts.dirs[name] = opt.Directory
	return t
}
func (ts *TemplateSet) Get(name string) *template.Template {
	ts.lock.RLock()
	defer ts.lock.RUnlock()
	return ts.sets[name]
}
func (ts *TemplateSet) GetDir(name string) string {
	ts.lock.RLock()
	defer ts.lock.RUnlock()
	return ts.dirs[name]
}
func compile(opt RenderOptions) *template.Template {
	t := template.New(opt.Directory)
	t.Delims(opt.Delims.Left, opt.Delims.Right)
	template.Must(t.Parse("ForChange")) // 解析初始模板,以防我们没有任何模板.
	if opt.TemplateFileSystem == nil {
		opt.TemplateFileSystem = NewTemplateFileSystem(opt, false)
	}
	for _, f := range opt.TemplateFileSystem.ListFiles() {
		tmpl := t.New(f.Name())
		for _, funcs := range opt.Funcs {
			tmpl.Funcs(funcs)
		}
		template.Must(tmpl.Funcs(helperFuncs).Parse(string(f.Data()))) // 如果解析失败会炸弹.我们不希望任何静默服务器启动.
	}
	return t
}

// 使用给定的选项创建新的模板文件系统.
func NewTemplateFileSystem(opt RenderOptions, omitData bool) TplFileSystem {
	fs := TplFileSystem{}
	fs.files = make([]TemplateFile, 0, 10)

	// 目录是按相反的顺序组成的，因为后面的目录会覆盖前面的目录，所以一旦找到，我们就可以直接跳出循环.
	dirs := make([]string, 0, len(opt.AppendDirectories)+1)
	for i := len(opt.AppendDirectories) - 1; i >= 0; i-- {
		dirs = append(dirs, opt.AppendDirectories[i])
	}
	dirs = append(dirs, opt.Directory)

	var err error
	for i := range dirs {
		// 跳过不存在的符号链接测试，但允许在启动后添加非符号链接的
		if !utils.IsExist(dirs[i]) {
			continue
		}

		dirs[i], err = filepath.EvalSymlinks(dirs[i])
		if err != nil {
			panic("EvalSymlinks(" + dirs[i] + "): " + err.Error())
		}
	}
	lastDir := dirs[len(dirs)-1]
	// 我们仍然遍历最后一个(原始)目录，因为加载原始目录中不存在的模板是没有意义的.
	if err = filepath.Walk(lastDir, func(path string, info os.FileInfo, err error) error {
		r, err := filepath.Rel(lastDir, path)
		if err != nil {
			return err
		}

		ext := ""
		index := strings.Index(r, ".")
		if index != -1 {
			ext = r[index:]
		}
		for _, extension := range opt.Extensions {
			if ext != extension {
				continue
			}
			var data []byte
			if !omitData {
				// 循环遍历目录的候选对象,一旦找到就会中断.
				// 该文件始终存在，因为它位于walk函数中,而读取原始文件是最坏的情况.
				for i := range dirs {
					path = filepath.Join(dirs[i], r)
					if !utils.IsFile(path) {
						continue
					}

					data, err = ioutil.ReadFile(path)
					if err != nil {
						return err
					}
					break
				}
			}

			name := filepath.ToSlash(r[0 : len(r)-len(ext)])
			fs.files = append(fs.files, NewTplFile(name, data, ext))
		}

		return nil
	}); err != nil {
		panic("NewTemplateFileSystem: " + err.Error())
	}

	return fs
}

// 实现TemplateFileSystem接口.
type TplFileSystem struct {
	files []TemplateFile
}

func (fs TplFileSystem) ListFiles() []TemplateFile {
	return fs.files
}

func (fs TplFileSystem) Get(name string) (io.Reader, error) {
	for i := range fs.files {
		if fs.files[i].Name()+fs.files[i].Ext() == name {
			return bytes.NewReader(fs.files[i].Data()), nil
		}
	}
	return nil, fmt.Errorf("file '%s' not found", name)
}

// 创建具有给定名称和数据的新模板文件.
func NewTplFile(name string, data []byte, ext string) *TplFile {
	return &TplFile{name, data, ext}
}

// 实现TemplateFile接口.
type TplFile struct {
	name string
	data []byte
	ext  string
}

func (f *TplFile) Name() string {
	return f.name
}

func (f *TplFile) Data() []byte {
	return f.data
}

func (f *TplFile) Ext() string {
	return f.ext
}

type TplRender struct {
	http.ResponseWriter
	*TemplateSet
	Opt       *RenderOptions
	Charset   string
	startTime time.Time
}

func (r *TplRender) HTML(status int, name string, data interface{}, htmlOpt ...HTMLOptions) {
	r.renderHTML(status, DefaultTplSetName, name, data, htmlOpt...)
}
func (r *TplRender) HTMLSet(status int, setName, tplName string, data interface{}, htmlOpt ...HTMLOptions) {
	r.renderHTML(status, setName, tplName, data, htmlOpt...)
}
func (r *TplRender) HTMLSetBytes(setName, tplName string, data interface{}, htmlOpt ...HTMLOptions) ([]byte, error) {
	out, err := r.renderBytes(setName, tplName, data, htmlOpt...)
	if err != nil {
		return []byte(""), err
	}
	return out.Bytes(), nil
}
func (r *TplRender) renderHTML(status int, setName, tplName string, data interface{}, htmlOpt ...HTMLOptions) {
	r.startTime = time.Now()
	out, err := r.renderBytes(setName, tplName, data, htmlOpt...)
	if err != nil {
		http.Error(r, err.Error(), http.StatusInternalServerError)
		return
	}
	r.Header().Set("Content-Type", r.Opt.HTMLContentType+r.Charset)
	r.WriteHeader(status)
	if _, err := out.WriteTo(r); err != nil {
		out.Reset()
	}
	bufPool.Put(out)
}
func (r *TplRender) renderBytes(setName, tplName string, data interface{}, htmlOpt ...HTMLOptions) (*bytes.Buffer, error) {
	t := r.TemplateSet.Get(setName)
	if TemplateEnv == "development" {
		opt := *r.Opt
		opt.Directory = r.TemplateSet.GetDir(setName)
		t = r.TemplateSet.Set(setName, &opt)
	}
	if t == nil {
		return nil, fmt.Errorf("html/template: template \"%s\" is undefined", tplName)
	}
	opt := r.prepareHTMLOptions(htmlOpt)
	if len(opt.Layout) > 0 {
		r.addYield(t, tplName, data)
		tplName = opt.Layout
	}
	out, err := r.execute(t, tplName, data)
	if err != nil {
		return nil, err
	}
	return out, nil
}
func (r *TplRender) prepareHTMLOptions(htmlOpt []HTMLOptions) HTMLOptions {
	if len(htmlOpt) > 0 {
		return htmlOpt[0]
	} else {
		return HTMLOptions{Layout: r.Opt.Layout}
	}
}
func (r *TplRender) addYield(t *template.Template, tplName string, data interface{}) {
	t.Funcs(template.FuncMap{
		"yield": func() (template.HTML, error) {
			buf, err := r.execute(t, tplName, data)
			return template.HTML(buf.String()), err // 在这里返回安全的html，因为我们正在渲染自己的模板
		},
		"current": func() (string, error) {
			return tplName, nil
		},
	})
}
func (r *TplRender) execute(t *template.Template, name string, data interface{}) (*bytes.Buffer, error) {
	buf := bufPool.Get().(*bytes.Buffer)
	return buf, t.ExecuteTemplate(buf, name, data)
}
func parseTplSet(tplSet string) (tplName string, tplDir string) {
	tplSet = strings.TrimSpace(tplSet)
	if len(tplSet) == 0 {
		panic("empty template set argument")
	}
	infos := strings.Split(tplSet, ":")
	if len(infos) == 1 {
		tplDir = infos[0]
		tplName = path.Base(tplDir)
	} else {
		tplName = infos[0]
		tplDir = infos[1]
	}

	if !utils.IsDir(tplDir) {
		panic("template set path does not exist or is not a directory")
	}
	return tplName, tplDir
}
func renderHandler(opt RenderOptions, tplSets []string) (*TemplateSet, RenderOptions, string) {
	cs := PrepareCharset(opt.Charset)
	ts := NewTemplateSet()
	ts.Set(DefaultTplSetName, &opt)
	var tmpOpt RenderOptions
	for _, tplSet := range tplSets {
		tplName, tplDir := parseTplSet(tplSet)
		tmpOpt = opt
		tmpOpt.Directory = tplDir
		ts.Set(tplName, &tmpOpt)
	}
	return ts, opt, cs
}
func prepareRenderOptions(options []RenderOptions) RenderOptions {
	var opt RenderOptions
	if len(options) > 0 {
		opt = options[0]
	}
	// Defaults.
	if len(opt.Directory) == 0 {
		opt.Directory = "templates"
	}
	if len(opt.Extensions) == 0 {
		opt.Extensions = []string{".tmpl", ".html"}
	}
	if len(opt.HTMLContentType) == 0 {
		opt.HTMLContentType = "text/html"
	}

	return opt
}

func PrepareCharset(charset string) string {
	if len(charset) != 0 {
		return "; charset=" + charset
	}

	return "; charset=\"UTF-8\""
}
