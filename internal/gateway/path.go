package gateway

import (
	"fmt"
	"strings"
)

type pathSegment struct {
	literal string
	isParam bool
}

func parsePathSegments(path string) ([]pathSegment, error) {
	if path == "" {
		return nil, fmt.Errorf("path cannot be empty")
	}
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("path must start with '/' (got %q)", path)
	}
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return []pathSegment{}, nil
	}
	parts := strings.Split(trimmed, "/")
	segments := make([]pathSegment, len(parts))
	for i, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("path %q contains empty segment", path)
		}
		if strings.Contains(part, "{") || strings.Contains(part, "}") {
			if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
				name := strings.TrimSpace(part[1 : len(part)-1])
				if name == "" {
					return nil, fmt.Errorf("path %q contains empty parameter name", path)
				}
				segments[i] = pathSegment{literal: name, isParam: true}
				continue
			}
			return nil, fmt.Errorf("path segment %q has unmatched braces", part)
		}
		segments[i] = pathSegment{literal: part, isParam: false}
	}
	return segments, nil
}

func extractPathParamNames(path string) ([]string, error) {
	segments, err := parsePathSegments(path)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, seg := range segments {
		if seg.isParam {
			names = append(names, seg.literal)
		}
	}
	return names, nil
}

type route struct {
	service  *Service
	endpoint Endpoint
	segments []pathSegment
}

func newRoute(svc *Service, ep Endpoint) (*route, error) {
	segments, err := parsePathSegments(ep.Path)
	if err != nil {
		return nil, err
	}
	return &route{
		service:  svc,
		endpoint: ep,
		segments: segments,
	}, nil
}

func (r *route) matchPath(requestPath string) (string, bool) {
	trimmed := strings.Trim(requestPath, "/")
	var parts []string
	if trimmed == "" {
		if len(r.segments) == 0 {
			return "/", true
		}
		return "", false
	}
	parts = strings.Split(trimmed, "/")
	if len(parts) != len(r.segments) {
		return "", false
	}
	matched := make([]string, len(parts))
	for i, seg := range r.segments {
		if seg.isParam {
			matched[i] = parts[i]
			continue
		}
		if seg.literal != parts[i] {
			return "", false
		}
		matched[i] = seg.literal
	}
	if len(matched) == 0 {
		return "/", true
	}
	return "/" + strings.Join(matched, "/"), true
}
