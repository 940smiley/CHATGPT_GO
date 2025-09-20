package gateway

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func (g *Gateway) BuildOpenAPISpec(baseURL string) ([]byte, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	spec := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "Local ChatGPT Gateway",
			"version":     "1.0.0",
			"description": "Auto-generated OpenAPI schema for locally discovered MCP servers.",
		},
		"servers": []any{
			map[string]any{"url": baseURL},
		},
		"paths": map[string]any{},
	}

	if len(g.services) > 0 {
		tags := make([]any, 0, len(g.services))
		serviceNames := make([]string, 0, len(g.services))
		for name := range g.services {
			serviceNames = append(serviceNames, name)
		}
		sort.Strings(serviceNames)
		for _, name := range serviceNames {
			svc := g.services[name]
			tag := map[string]any{"name": svc.Name}
			if svc.Description != "" {
				tag["description"] = svc.Description
			}
			tags = append(tags, tag)
		}
		spec["tags"] = tags
	}

	paths := spec["paths"].(map[string]any)

	serviceNames := make([]string, 0, len(g.services))
	for name := range g.services {
		serviceNames = append(serviceNames, name)
	}
	sort.Strings(serviceNames)

	for _, name := range serviceNames {
		svc := g.services[name]
		for _, ep := range svc.Endpoints {
			method := strings.ToLower(ep.Method)
			if method == "" {
				continue
			}
			pathItem, _ := paths[ep.Path].(map[string]any)
			if pathItem == nil {
				pathItem = make(map[string]any)
				paths[ep.Path] = pathItem
			}
			summary := ep.Description
			if summary == "" {
				summary = fmt.Sprintf("%s %s", strings.ToUpper(method), ep.Path)
			}
			operation := map[string]any{
				"summary":     summary,
				"description": buildOperationDescription(svc, ep),
				"tags":        []string{svc.Name},
				"responses": map[string]any{
					"200":     map[string]any{"description": "Successful response."},
					"default": map[string]any{"description": "Unexpected error."},
				},
				"x-service-name":    svc.Name,
				"x-service-address": svc.Address,
			}
			operationID := ep.OperationID
			if operationID == "" {
				operationID = generateOperationID(svc.Name, method, ep.Path)
			}
			operation["operationId"] = operationID

			if len(ep.Parameters) > 0 {
				operation["parameters"] = convertParameters(ep.Parameters)
			}
			if body := convertRequestBody(ep.RequestBody); body != nil {
				operation["requestBody"] = body
			}
			pathItem[method] = operation
		}
	}

	return json.MarshalIndent(spec, "", "  ")
}

func buildOperationDescription(svc *Service, ep Endpoint) string {
	var parts []string
	if ep.Description != "" {
		parts = append(parts, ep.Description)
	}
	if svc.Description != "" {
		parts = append(parts, fmt.Sprintf("Service description: %s", svc.Description))
	}
	parts = append(parts, fmt.Sprintf("Requests are proxied to %s%s", svc.Address, ep.Path))
	return strings.Join(parts, "\n\n")
}

func convertParameters(params []Parameter) []any {
	if len(params) == 0 {
		return nil
	}
	sorted := make([]Parameter, len(params))
	copy(sorted, params)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].In == sorted[j].In {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].In < sorted[j].In
	})
	result := make([]any, 0, len(sorted))
	for _, p := range sorted {
		item := map[string]any{
			"name":     p.Name,
			"in":       p.In,
			"required": p.Required,
		}
		if p.Description != "" {
			item["description"] = p.Description
		}
		if len(p.Schema) > 0 {
			item["schema"] = p.Schema
		}
		result = append(result, item)
	}
	return result
}

func convertRequestBody(rb *RequestBody) map[string]any {
	if rb == nil {
		return nil
	}
	content := make(map[string]any)
	for mediaType, def := range rb.Content {
		if def.Schema == nil && def.Example == nil {
			continue
		}
		media := make(map[string]any)
		if def.Schema != nil {
			media["schema"] = def.Schema
		}
		if def.Example != nil {
			media["example"] = def.Example
		}
		content[mediaType] = media
	}
	if len(content) == 0 {
		return nil
	}
	body := map[string]any{
		"content": content,
	}
	if rb.Description != "" {
		body["description"] = rb.Description
	}
	if rb.Required {
		body["required"] = true
	}
	return body
}

func generateOperationID(serviceName, method, path string) string {
	sanitized := strings.ReplaceAll(path, "/", "_")
	sanitized = strings.ReplaceAll(sanitized, "{", "")
	sanitized = strings.ReplaceAll(sanitized, "}", "")
	sanitized = strings.ReplaceAll(sanitized, "-", "_")
	sanitized = strings.ReplaceAll(sanitized, " ", "_")
	sanitized = strings.ReplaceAll(sanitized, ".", "_")
	sanitized = strings.ReplaceAll(sanitized, "__", "_")
	sanitized = strings.Trim(sanitized, "_")
	if sanitized == "" {
		sanitized = "root"
	}
	return fmt.Sprintf("%s_%s_%s", serviceName, method, sanitized)
}
