package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Site     SiteConfig       `yaml:"site"`
	Defaults DefaultsConfig   `yaml:"defaults"`
	Indexes  map[string]Index `yaml:"indexes"`
	Tasks    map[string]Task  `yaml:"tasks"`
}

type SiteConfig struct {
	ID      string `yaml:"id"`
	BaseURL string `yaml:"base_url"`
}

type DefaultsConfig struct {
	Headers   map[string]string `yaml:"headers"`
	Cookie    string            `yaml:"cookie"`
	Autoclick *AutoclickConfig  `yaml:"autoclick"`
}

type Task struct {
	Params        map[string]ParamSpec     `yaml:"params"`
	ResolveParams map[string]ParamResolver `yaml:"resolve_params"`
	Request       RequestConfig            `yaml:"request"`
	Headers       map[string]string        `yaml:"headers"`
	Extract       ExtractConfig            `yaml:"extract"`
	Pagination    *PaginationConfig        `yaml:"pagination"`
	Output        OutputConfig             `yaml:"output"`
	Enhance       EnhanceConfig            `yaml:"enhance"`
}

type ParamSpec struct {
	Required   bool   `yaml:"required"`
	Default    string `yaml:"default"`
	Regex      string `yaml:"regex"`
	RegexGroup int    `yaml:"regex_group"`
}

type Index struct {
	Path          string                   `yaml:"path"`
	Items         []map[string]interface{} `yaml:"items"`
	ItemsKey      string                   `yaml:"items_key"`
	MatchField    string                   `yaml:"match_field"`
	ValueField    string                   `yaml:"value_field"`
	CaseSensitive bool                     `yaml:"case_sensitive"`
}

type ParamResolver struct {
	Index      string `yaml:"index"`
	From       string `yaml:"from"`
	MatchField string `yaml:"match_field"`
	ValueField string `yaml:"value_field"`
	Optional   bool   `yaml:"optional"`
}

type RequestConfig struct {
	Method         string            `yaml:"method"`
	URL            string            `yaml:"url"`
	Path           string            `yaml:"path"`
	Query          map[string]string `yaml:"query"`
	OmitEmptyQuery bool              `yaml:"omit_empty_query"`
	AcceptStatus   []int             `yaml:"accept_status"`
	Autoclick      *AutoclickConfig  `yaml:"autoclick"`
}

type AutoclickConfig struct {
	XPath string `yaml:"xpath"`
}

type ExtractConfig struct {
	Scope  *ScopeConfig           `yaml:"scope"`
	Meta   map[string]FieldConfig `yaml:"meta"`
	Page   map[string]FieldConfig `yaml:"page"`
	Fields map[string]FieldConfig `yaml:"fields"`
}

type ScopeConfig struct {
	XPath string `yaml:"xpath"`
}

type FieldConfig struct {
	XPath      string `yaml:"xpath"`
	Attr       string `yaml:"attr"`
	Regex      string `yaml:"regex"`
	RegexGroup int    `yaml:"regex_group"`
	Type       string `yaml:"type"`
	Trim       bool   `yaml:"trim"`
	Multiple   bool   `yaml:"multiple"`
	Required   bool   `yaml:"required"`
	Default    string `yaml:"default"`
	OnMissing  string `yaml:"on_missing"`
	ResolveURL bool   `yaml:"resolve_url"`
}

type PaginationConfig struct {
	Param      string `yaml:"param"`
	Default    string `yaml:"default"`
	TotalField string `yaml:"total_field"`
}

type OutputConfig struct {
	Type       string                 `yaml:"type"`
	ItemsKey   string                 `yaml:"items_key"`
	PageFormat map[string]interface{} `yaml:"page_format"`
	Format     map[string]interface{} `yaml:"format"`
}

type EnhanceConfig struct {
	ActorImage *ActorImageEnhanceConfig `yaml:"actor_image"`
}

type ActorImageEnhanceConfig struct {
	Source     string `yaml:"source"`
	ItemsKey   string `yaml:"items_key"`
	NameField  string `yaml:"name_field"`
	ImageField string `yaml:"image_field"`
}

func Load(path string) (*Config, error) {
	if isBuiltinRef(path) {
		return LoadBuiltin(path)
	}
	return LoadFile(path)
}

func LoadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

func Parse(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func isBuiltinRef(value string) bool {
	ref := strings.TrimSpace(value)
	if ref == "" {
		return false
	}
	return !strings.ContainsAny(ref, `/\`) && !strings.HasSuffix(ref, ".yml") && !strings.HasSuffix(ref, ".yaml")
}

func (c *Config) Validate() error {
	if strings.TrimSpace(c.Site.ID) == "" {
		return errors.New("site.id is required")
	}
	if strings.TrimSpace(c.Site.BaseURL) == "" {
		return errors.New("site.base_url is required")
	}
	if len(c.Tasks) == 0 {
		return errors.New("tasks must contain at least one task")
	}
	if err := validateAutoclick("defaults.autoclick", c.Defaults.Autoclick); err != nil {
		return err
	}
	for name, task := range c.Tasks {
		if err := c.validateTask(name, task); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) validateTask(name string, task Task) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("task name cannot be empty")
	}
	if strings.TrimSpace(task.Request.Method) == "" {
		task.Request.Method = "GET"
	}
	if strings.TrimSpace(task.Request.URL) == "" && strings.TrimSpace(task.Request.Path) == "" {
		return fmt.Errorf("task %q: request.url or request.path is required", name)
	}
	if err := validateAutoclick(fmt.Sprintf("task %q: request.autoclick", name), task.Request.Autoclick); err != nil {
		return err
	}
	if len(task.Extract.Fields) == 0 {
		return fmt.Errorf("task %q: extract.fields must contain at least one field", name)
	}
	if strings.TrimSpace(task.Output.Type) == "" {
		task.Output.Type = "list"
	}
	if len(task.Output.Format) == 0 {
		return fmt.Errorf("task %q: output.format must contain at least one field", name)
	}
	if task.Pagination != nil {
		if strings.TrimSpace(task.Pagination.Param) == "" {
			return fmt.Errorf("task %q: pagination.param is required", name)
		}
	}
	if task.Enhance.ActorImage != nil {
		source := strings.ToLower(strings.TrimSpace(task.Enhance.ActorImage.Source))
		if source == "" {
			return fmt.Errorf("task %q: enhance.actor_image.source is required", name)
		}
		if source != "gfriends" {
			return fmt.Errorf("task %q: unsupported enhance.actor_image.source %q", name, task.Enhance.ActorImage.Source)
		}
	}
	for target, resolver := range task.ResolveParams {
		if strings.TrimSpace(target) == "" {
			return fmt.Errorf("task %q: resolve_params key cannot be empty", name)
		}
		if strings.TrimSpace(resolver.Index) == "" {
			return fmt.Errorf("task %q: resolve_params.%s.index is required", name, target)
		}
		if strings.TrimSpace(resolver.From) == "" {
			return fmt.Errorf("task %q: resolve_params.%s.from is required", name, target)
		}
		if _, ok := c.Indexes[resolver.Index]; !ok {
			return fmt.Errorf("task %q: resolve_params.%s references unknown index %q", name, target, resolver.Index)
		}
	}
	for paramName, spec := range task.Params {
		if strings.TrimSpace(spec.Regex) == "" {
			continue
		}
		if _, err := regexp.Compile(spec.Regex); err != nil {
			return fmt.Errorf("task %q: params.%s.regex: %w", name, paramName, err)
		}
		if spec.RegexGroup < 0 {
			return fmt.Errorf("task %q: params.%s.regex_group cannot be negative", name, paramName)
		}
	}
	return nil
}

func validateAutoclick(label string, autoclick *AutoclickConfig) error {
	if autoclick == nil {
		return nil
	}
	if strings.TrimSpace(autoclick.XPath) == "" {
		return fmt.Errorf("%s.xpath is required", label)
	}
	return nil
}

func (c *Config) Task(name string) (Task, error) {
	task, ok := c.Tasks[name]
	if !ok {
		return Task{}, fmt.Errorf("task %q not found", name)
	}
	return task, nil
}

func (c *Config) HeadersFor(task Task) map[string]string {
	headers := make(map[string]string, len(c.Defaults.Headers)+len(task.Headers))
	for k, v := range c.Defaults.Headers {
		headers[k] = v
	}
	for k, v := range task.Headers {
		headers[k] = v
	}
	return headers
}
