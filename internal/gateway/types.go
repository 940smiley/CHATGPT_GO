package gateway

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Service struct {
	Name        string     `yaml:"serviceName"`
	Address     string     `yaml:"serviceAddress"`
	Description string     `yaml:"description"`
	Endpoints   []Endpoint `yaml:"endpoints"`
	Source      string     `yaml:"-"`
}

type Endpoint struct {
	Path        string       `yaml:"path"`
	Method      string       `yaml:"method"`
	Description string       `yaml:"description"`
	OperationID string       `yaml:"operationId"`
	Parameters  []Parameter  `yaml:"parameters"`
	RequestBody *RequestBody `yaml:"requestBody"`
}

type Parameter struct {
	Name        string         `yaml:"name"`
	In          string         `yaml:"in"`
	Required    bool           `yaml:"required"`
	Description string         `yaml:"description"`
	Schema      map[string]any `yaml:"schema"`
}

type RequestBody struct {
	Description string                         `yaml:"description"`
	Required    bool                           `yaml:"required"`
	Content     map[string]MediaTypeDefinition `yaml:"content"`
}

type MediaTypeDefinition struct {
	Schema  map[string]any `yaml:"schema"`
	Example any            `yaml:"example"`
}

func LoadService(path string) (*Service, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory, expected a YAML file", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var svc Service
	if err := yaml.Unmarshal(data, &svc); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", filepath.Base(path), err)
	}
	svc.Source = path
	if err := svc.normalizeAndValidate(); err != nil {
		return nil, fmt.Errorf("invalid service definition %s: %w", filepath.Base(path), err)
	}
	return &svc, nil
}

func (s *Service) normalizeAndValidate() error {
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		return fmt.Errorf("serviceName is required")
	}
	s.Address = strings.TrimSpace(s.Address)
	if s.Address == "" {
		return fmt.Errorf("serviceAddress is required")
	}
	s.Address = strings.TrimRight(s.Address, "/")
	s.Description = strings.TrimSpace(s.Description)
	if len(s.Endpoints) == 0 {
		return fmt.Errorf("service must define at least one endpoint")
	}
	for i := range s.Endpoints {
		ep := &s.Endpoints[i]
		ep.Path = strings.TrimSpace(ep.Path)
		if ep.Path == "" {
			return fmt.Errorf("endpoint %d path is required", i)
		}
		if !strings.HasPrefix(ep.Path, "/") {
			ep.Path = "/" + ep.Path
		}
		method := strings.ToUpper(strings.TrimSpace(ep.Method))
		if method == "" {
			return fmt.Errorf("endpoint %s must define a method", ep.Path)
		}
		ep.Method = method
		ep.Description = strings.TrimSpace(ep.Description)
		ep.OperationID = strings.TrimSpace(ep.OperationID)

		paramsInPath, err := extractPathParamNames(ep.Path)
		if err != nil {
			return fmt.Errorf("endpoint %s %s has invalid path: %w", ep.Method, ep.Path, err)
		}
		expectedParams := make(map[string]bool, len(paramsInPath))
		for _, name := range paramsInPath {
			expectedParams[name] = false
		}

		for idx := range ep.Parameters {
			p := &ep.Parameters[idx]
			p.Name = strings.TrimSpace(p.Name)
			if p.Name == "" {
				return fmt.Errorf("endpoint %s %s has a parameter with an empty name", ep.Method, ep.Path)
			}
			inValue := strings.ToLower(strings.TrimSpace(p.In))
			if inValue == "" {
				if _, ok := expectedParams[p.Name]; ok {
					inValue = "path"
				} else {
					inValue = "query"
				}
			}
			p.In = inValue
			if p.In == "path" {
				p.Required = true
				if _, ok := expectedParams[p.Name]; ok {
					expectedParams[p.Name] = true
				}
			}
			p.Description = strings.TrimSpace(p.Description)
			if p.Schema == nil {
				p.Schema = map[string]any{"type": "string"}
			}
		}

		for name, found := range expectedParams {
			if !found {
				ep.Parameters = append(ep.Parameters, Parameter{
					Name:     name,
					In:       "path",
					Required: true,
					Schema:   map[string]any{"type": "string"},
				})
			}
		}

		if ep.RequestBody != nil {
			ep.RequestBody.Description = strings.TrimSpace(ep.RequestBody.Description)
			if len(ep.RequestBody.Content) == 0 {
				ep.RequestBody = nil
			}
		}
	}
	return nil
}
