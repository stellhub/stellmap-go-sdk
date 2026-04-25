package registry

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ComposeServiceName 组合五段式服务标识。
func ComposeServiceName(organization, businessDomain, capabilityDomain, application, role string) string {
	parts := []string{
		strings.TrimSpace(organization),
		strings.TrimSpace(businessDomain),
		strings.TrimSpace(capabilityDomain),
		strings.TrimSpace(application),
		strings.TrimSpace(role),
	}
	return strings.Join(parts, ".")
}

// ParseServiceName 解析五段式服务标识。
func ParseServiceName(service string) (organization, businessDomain, capabilityDomain, application, role string, ok bool) {
	service = strings.TrimSpace(service)
	if service == "" {
		return "", "", "", "", "", false
	}

	parts := strings.Split(service, ".")
	if len(parts) != 5 {
		return "", "", "", "", "", false
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return "", "", "", "", "", false
		}
	}

	return parts[0], parts[1], parts[2], parts[3], parts[4], true
}

// EffectiveLeaseTTLSeconds 返回有效租约 TTL。
func EffectiveLeaseTTLSeconds(ttl int64) int64 {
	if ttl > 0 {
		return ttl
	}
	return DefaultLeaseTTLSeconds
}

// EffectiveHeartbeatInterval 返回推荐心跳周期。
func EffectiveHeartbeatInterval(ttl int64) time.Duration {
	effectiveTTL := EffectiveLeaseTTLSeconds(ttl)
	if effectiveTTL <= 3 {
		return time.Second
	}
	interval := effectiveTTL / 3
	if interval <= 0 {
		interval = 1
	}
	return time.Duration(interval) * time.Second
}

func normalizeRegisterRequest(request RegisterRequest) (RegisterRequest, error) {
	request.Namespace = strings.TrimSpace(request.Namespace)
	request.InstanceID = strings.TrimSpace(request.InstanceID)
	request.Zone = strings.TrimSpace(request.Zone)
	request.Labels = cloneStringMap(request.Labels)
	request.Metadata = cloneStringMap(request.Metadata)

	if err := normalizeStructuredServiceIdentity(
		&request.Service,
		&request.Organization,
		&request.BusinessDomain,
		&request.CapabilityDomain,
		&request.Application,
		&request.Role,
	); err != nil {
		return RegisterRequest{}, err
	}
	if request.Namespace == "" || request.Service == "" || request.InstanceID == "" {
		return RegisterRequest{}, fmt.Errorf("namespace, service and instanceId are required")
	}
	if request.LeaseTTLSeconds < 0 {
		return RegisterRequest{}, fmt.Errorf("leaseTtlSeconds must be greater than or equal to 0")
	}
	request.LeaseTTLSeconds = EffectiveLeaseTTLSeconds(request.LeaseTTLSeconds)
	if len(request.Endpoints) == 0 {
		return RegisterRequest{}, fmt.Errorf("at least one endpoint is required")
	}

	seen := make(map[string]struct{}, len(request.Endpoints))
	normalizedEndpoints := make([]Endpoint, 0, len(request.Endpoints))
	for _, endpoint := range request.Endpoints {
		endpoint.Name = strings.TrimSpace(endpoint.Name)
		endpoint.Protocol = strings.TrimSpace(endpoint.Protocol)
		endpoint.Host = strings.TrimSpace(endpoint.Host)
		endpoint.Path = strings.TrimSpace(endpoint.Path)

		if endpoint.Protocol == "" {
			return RegisterRequest{}, fmt.Errorf("endpoint protocol is required")
		}
		if endpoint.Name == "" {
			endpoint.Name = endpoint.Protocol
		}
		if endpoint.Host == "" {
			return RegisterRequest{}, fmt.Errorf("endpoint host is required")
		}
		if endpoint.Port <= 0 || endpoint.Port > 65535 {
			return RegisterRequest{}, fmt.Errorf("endpoint port must be within 1..65535")
		}
		if endpoint.Weight < 0 {
			return RegisterRequest{}, fmt.Errorf("endpoint weight must be greater than or equal to 0")
		}
		if endpoint.Weight == 0 {
			endpoint.Weight = DefaultEndpointWeight
		}
		if _, ok := seen[endpoint.Name]; ok {
			return RegisterRequest{}, fmt.Errorf("duplicate endpoint name: %s", endpoint.Name)
		}
		seen[endpoint.Name] = struct{}{}
		normalizedEndpoints = append(normalizedEndpoints, endpoint)
	}
	request.Endpoints = normalizedEndpoints

	return request, nil
}

func normalizeDeregisterRequest(request DeregisterRequest) (DeregisterRequest, error) {
	request.Namespace = strings.TrimSpace(request.Namespace)
	request.InstanceID = strings.TrimSpace(request.InstanceID)

	if err := normalizeStructuredServiceIdentity(
		&request.Service,
		&request.Organization,
		&request.BusinessDomain,
		&request.CapabilityDomain,
		&request.Application,
		&request.Role,
	); err != nil {
		return DeregisterRequest{}, err
	}
	if request.Namespace == "" || request.Service == "" || request.InstanceID == "" {
		return DeregisterRequest{}, fmt.Errorf("namespace, service and instanceId are required")
	}
	return request, nil
}

func normalizeHeartbeatRequest(request HeartbeatRequest) (HeartbeatRequest, error) {
	request.Namespace = strings.TrimSpace(request.Namespace)
	request.InstanceID = strings.TrimSpace(request.InstanceID)

	if err := normalizeStructuredServiceIdentity(
		&request.Service,
		&request.Organization,
		&request.BusinessDomain,
		&request.CapabilityDomain,
		&request.Application,
		&request.Role,
	); err != nil {
		return HeartbeatRequest{}, err
	}
	if request.Namespace == "" || request.Service == "" || request.InstanceID == "" {
		return HeartbeatRequest{}, fmt.Errorf("namespace, service and instanceId are required")
	}
	if request.LeaseTTLSeconds < 0 {
		return HeartbeatRequest{}, fmt.Errorf("leaseTtlSeconds must be greater than or equal to 0")
	}
	if request.LeaseTTLSeconds > 0 {
		request.LeaseTTLSeconds = EffectiveLeaseTTLSeconds(request.LeaseTTLSeconds)
	}
	return request, nil
}

func normalizeQueryOptions(options QueryOptions) (QueryOptions, error) {
	options.Namespace = strings.TrimSpace(options.Namespace)
	options.Service = strings.TrimSpace(options.Service)
	options.ServicePrefix = strings.TrimSpace(options.ServicePrefix)
	options.Organization = strings.TrimSpace(options.Organization)
	options.BusinessDomain = strings.TrimSpace(options.BusinessDomain)
	options.CapabilityDomain = strings.TrimSpace(options.CapabilityDomain)
	options.Application = strings.TrimSpace(options.Application)
	options.Role = strings.TrimSpace(options.Role)
	options.Zone = strings.TrimSpace(options.Zone)
	options.Endpoint = strings.TrimSpace(options.Endpoint)
	options.Services = normalizeStringSlice(append(options.Services, options.Service))
	options.ServicePrefixes = normalizeStringSlice(append(options.ServicePrefixes, options.ServicePrefix))
	options.Selectors = normalizeStringSlice(options.Selectors)
	options.Labels = normalizeStringSlice(options.Labels)

	if options.Namespace == "" {
		return QueryOptions{}, fmt.Errorf("namespace is required")
	}
	if options.Limit < 0 {
		return QueryOptions{}, fmt.Errorf("limit must be greater than or equal to 0")
	}

	service := options.Service
	organization := options.Organization
	businessDomain := options.BusinessDomain
	capabilityDomain := options.CapabilityDomain
	application := options.Application
	role := options.Role

	if service != "" {
		parsedOrganization, parsedBusinessDomain, parsedCapabilityDomain, parsedApplication, parsedRole, ok := ParseServiceName(service)
		if ok {
			if organization == "" {
				organization = parsedOrganization
			}
			if businessDomain == "" {
				businessDomain = parsedBusinessDomain
			}
			if capabilityDomain == "" {
				capabilityDomain = parsedCapabilityDomain
			}
			if application == "" {
				application = parsedApplication
			}
			if role == "" {
				role = parsedRole
			}
		}
	}

	if hasAnyServiceDimension(organization, businessDomain, capabilityDomain, application, role) {
		if hasGapInServiceDimensions(organization, businessDomain, capabilityDomain, application, role) {
			return QueryOptions{}, fmt.Errorf("service hierarchy filters must be contiguous from organization to role")
		}
		if hasAllServiceDimensions(organization, businessDomain, capabilityDomain, application, role) {
			canonical := ComposeServiceName(organization, businessDomain, capabilityDomain, application, role)
			if service != "" && service != canonical {
				return QueryOptions{}, fmt.Errorf("service %q does not match structured service identity %q", service, canonical)
			}
			options.Service = canonical
			options.Services = normalizeStringSlice(append(options.Services, canonical))
		} else {
			prefix := strings.Join(serviceHierarchyDimensions(organization, businessDomain, capabilityDomain, application, role), ".")
			options.ServicePrefixes = normalizeStringSlice(append(options.ServicePrefixes, prefix))
		}
	}

	if len(options.Services) == 1 {
		options.Service = options.Services[0]
	}
	if len(options.ServicePrefixes) == 1 {
		options.ServicePrefix = options.ServicePrefixes[0]
	}

	return options, nil
}

func buildQueryValues(options QueryOptions) (url.Values, error) {
	options, err := normalizeQueryOptions(options)
	if err != nil {
		return nil, err
	}

	values := url.Values{}
	values.Set("namespace", options.Namespace)
	for _, service := range options.Services {
		values.Add("service", service)
	}
	for _, prefix := range options.ServicePrefixes {
		values.Add("servicePrefix", prefix)
	}
	addQueryValue(values, "organization", options.Organization)
	addQueryValue(values, "businessDomain", options.BusinessDomain)
	addQueryValue(values, "capabilityDomain", options.CapabilityDomain)
	addQueryValue(values, "application", options.Application)
	addQueryValue(values, "role", options.Role)
	addQueryValue(values, "zone", options.Zone)
	addQueryValue(values, "endpoint", options.Endpoint)
	for _, selector := range options.Selectors {
		values.Add("selector", selector)
	}
	for _, label := range options.Labels {
		values.Add("label", label)
	}
	if options.Limit > 0 {
		values.Set("limit", strconv.Itoa(options.Limit))
	}

	return values, nil
}

// BuildQueryValues 把查询参数编码为 URL Query。
func BuildQueryValues(options QueryOptions) (url.Values, error) {
	return buildQueryValues(options)
}

func callerHeaders(identity *CallerIdentity) (map[string]string, error) {
	if identity == nil {
		return nil, nil
	}

	service := strings.TrimSpace(identity.Service)
	organization := strings.TrimSpace(identity.Organization)
	businessDomain := strings.TrimSpace(identity.BusinessDomain)
	capabilityDomain := strings.TrimSpace(identity.CapabilityDomain)
	application := strings.TrimSpace(identity.Application)
	role := strings.TrimSpace(identity.Role)

	if err := normalizeStructuredServiceIdentity(
		&service,
		&organization,
		&businessDomain,
		&capabilityDomain,
		&application,
		&role,
	); err != nil {
		return nil, fmt.Errorf("invalid caller identity: %w", err)
	}

	headers := map[string]string{}
	addHeaderValue(headers, "X-StellMap-Caller-Namespace", identity.Namespace)
	addHeaderValue(headers, "X-StellMap-Caller-Service", service)
	addHeaderValue(headers, "X-StellMap-Caller-Organization", organization)
	addHeaderValue(headers, "X-StellMap-Caller-Business-Domain", businessDomain)
	addHeaderValue(headers, "X-StellMap-Caller-Capability-Domain", capabilityDomain)
	addHeaderValue(headers, "X-StellMap-Caller-Application", application)
	addHeaderValue(headers, "X-StellMap-Caller-Role", role)
	return headers, nil
}

func normalizeStructuredServiceIdentity(service, organization, businessDomain, capabilityDomain, application, role *string) error {
	*service = strings.TrimSpace(*service)
	*organization = strings.TrimSpace(*organization)
	*businessDomain = strings.TrimSpace(*businessDomain)
	*capabilityDomain = strings.TrimSpace(*capabilityDomain)
	*application = strings.TrimSpace(*application)
	*role = strings.TrimSpace(*role)

	if *service != "" {
		parsedOrganization, parsedBusinessDomain, parsedCapabilityDomain, parsedApplication, parsedRole, _ := ParseServiceName(*service)
		if *organization == "" {
			*organization = parsedOrganization
		}
		if *businessDomain == "" {
			*businessDomain = parsedBusinessDomain
		}
		if *capabilityDomain == "" {
			*capabilityDomain = parsedCapabilityDomain
		}
		if *application == "" {
			*application = parsedApplication
		}
		if *role == "" {
			*role = parsedRole
		}
		if !hasAnyServiceDimension(*organization, *businessDomain, *capabilityDomain, *application, *role) {
			return nil
		}
	}

	if !hasAnyServiceDimension(*organization, *businessDomain, *capabilityDomain, *application, *role) {
		if *service == "" {
			return fmt.Errorf("service is required")
		}
		return nil
	}
	if !hasAllServiceDimensions(*organization, *businessDomain, *capabilityDomain, *application, *role) {
		return fmt.Errorf("organization, businessDomain, capabilityDomain, application and role are required")
	}

	canonical := ComposeServiceName(*organization, *businessDomain, *capabilityDomain, *application, *role)
	if *service == "" {
		*service = canonical
		return nil
	}
	if *service != canonical {
		return fmt.Errorf("service %q does not match structured service identity %q", *service, canonical)
	}
	return nil
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}

	target := make(map[string]string, len(source))
	for key, value := range source {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		target[key] = strings.TrimSpace(value)
	}
	if len(target) == 0 {
		return nil
	}
	return target
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func addQueryValue(values url.Values, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	values.Set(key, value)
}

func addHeaderValue(headers map[string]string, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	headers[key] = value
}

func hasAnyServiceDimension(values ...string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func hasAllServiceDimensions(values ...string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return false
		}
	}
	return len(values) > 0
}

func hasGapInServiceDimensions(values ...string) bool {
	seenEmpty := false
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			seenEmpty = true
			continue
		}
		if seenEmpty {
			return true
		}
	}
	return false
}

func serviceHierarchyDimensions(organization, businessDomain, capabilityDomain, application, role string) []string {
	values := []string{organization, businessDomain, capabilityDomain, application, role}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			break
		}
		result = append(result, value)
	}
	return result
}
