package user

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const UsernameValidationMessage = "Username only can be 3 to 32 characters and contain a-z, 0-9, underscores, hyphens, dots, or @."

var (
	usernameRegexp    = regexp.MustCompile(`^[a-zA-Z0-9._@-]+$`)
	telegramRegexp    = regexp.MustCompile(`^[\w.@+\- ]+$`)
	contactRegexp     = regexp.MustCompile(`^[0-9+\-() ]+$`)
	autoServiceRegexp = regexp.MustCompile(`(?i)^setservice-(\d+)$`)
)

type ValidationError struct {
	Detail string
}

func (e ValidationError) Error() string {
	return e.Detail
}

func ValidateUserCreate(payload *UserCreate, catalog MutationContext) error {
	if payload == nil {
		return ValidationError{Detail: "payload is required"}
	}
	if err := validateUsername(payload.Username); err != nil {
		return err
	}
	if payload.Status != "" && payload.Status != UserStatusCreateActive && payload.Status != UserStatusCreateOnHold {
		return ValidationError{Detail: "invalid user status"}
	}
	if err := validateUserBase(&payload.UserPayloadBase, true, catalog); err != nil {
		return err
	}
	return validateOnHoldCreate(payload.Status, payload.OnHoldExpireDuration, payload.Expire)
}

func ValidateUserServiceCreate(payload *UserServiceCreate, catalog MutationContext) error {
	if payload == nil {
		return ValidationError{Detail: "payload is required"}
	}
	if err := validateUsername(payload.Username); err != nil {
		return err
	}
	if payload.ServiceID <= 0 {
		return ValidationError{Detail: "service_id must be a positive integer"}
	}
	if payload.Status != "" && payload.Status != UserStatusCreateActive && payload.Status != UserStatusCreateOnHold {
		return ValidationError{Detail: "invalid user status"}
	}
	base := UserPayloadBase{
		CredentialKey:          payload.CredentialKey,
		Flow:                   payload.Flow,
		Expire:                 payload.Expire,
		DataLimit:              payload.DataLimit,
		DataLimitResetStrategy: payload.DataLimitResetStrategy,
		Note:                   payload.Note,
		TelegramID:             nil,
		ContactNumber:          nil,
		OnHoldExpireDuration:   payload.OnHoldExpireDuration,
		OnHoldTimeout:          payload.OnHoldTimeout,
		IPLimit:                payload.IPLimit,
		AutoDeleteInDays:       payload.AutoDeleteInDays,
		NextPlan:               payload.NextPlan,
		NextPlans:              payload.NextPlans,
	}
	if err := validateUserBase(&base, false, catalog); err != nil {
		return err
	}
	return validateOnHoldCreate(payload.Status, payload.OnHoldExpireDuration, payload.Expire)
}

func ValidateUserModify(payload *UserModify, catalog MutationContext) error {
	if payload == nil {
		return ValidationError{Detail: "payload is required"}
	}
	if payload.Status != "" && payload.Status != UserStatusModifyActive && payload.Status != UserStatusModifyDisabled && payload.Status != UserStatusModifyOnHold {
		return ValidationError{Detail: "invalid user status"}
	}
	if payload.ServiceID != nil && *payload.ServiceID <= 0 {
		return ValidationError{Detail: "service_id must be a positive integer"}
	}
	if err := validateUserBase(&payload.UserPayloadBase, false, catalog); err != nil {
		return err
	}
	return validateOnHoldModify(payload.Status, payload.OnHoldExpireDuration, payload.Expire)
}

func ValidateBulkUsersAction(payload *BulkUsersActionRequest) error {
	if payload == nil {
		return ValidationError{Detail: "payload is required"}
	}
	needsDays := map[AdvancedUserAction]struct{}{
		AdvancedUserActionExtendExpire:  {},
		AdvancedUserActionReduceExpire:  {},
		AdvancedUserActionCleanupStatus: {},
	}
	if _, ok := needsDays[payload.Action]; ok {
		if payload.Days == nil || *payload.Days <= 0 {
			return ValidationError{Detail: "days must be a positive integer"}
		}
	}
	if payload.Action == AdvancedUserActionIncreaseTraffic || payload.Action == AdvancedUserActionDecreaseTraffic {
		if payload.Gigabytes == nil || *payload.Gigabytes <= 0 {
			return ValidationError{Detail: "gigabytes must be a positive number"}
		}
	}
	if payload.Action == AdvancedUserActionCleanupStatus {
		allowed := map[UserStatus]struct{}{UserStatusExpired: {}, UserStatusLimited: {}}
		statuses := payload.Statuses
		if len(statuses) == 0 {
			statuses = []UserStatus{UserStatusExpired, UserStatusLimited}
		}
		for _, status := range statuses {
			if _, ok := allowed[status]; !ok {
				return ValidationError{Detail: "cleanup_status only accepts expired or limited"}
			}
		}
		payload.Statuses = statuses
	}
	if len(payload.Scope) > 0 {
		cleaned := make([]UserStatus, 0, len(payload.Scope))
		seen := map[UserStatus]struct{}{}
		for _, status := range payload.Scope {
			if status == UserStatusDeleted {
				continue
			}
			if _, ok := seen[status]; ok {
				continue
			}
			seen[status] = struct{}{}
			cleaned = append(cleaned, status)
		}
		if len(cleaned) == 0 {
			return ValidationError{Detail: "scope cannot be empty or include deleted"}
		}
		payload.Scope = cleaned
	}
	if payload.ServiceID != nil && *payload.ServiceID <= 0 {
		return ValidationError{Detail: "service_id must be a positive integer"}
	}
	if payload.Action == AdvancedUserActionChangeService && payload.TargetServiceID != nil && *payload.TargetServiceID <= 0 {
		return ValidationError{Detail: "target_service_id must be a positive integer when provided for change_service"}
	}
	if payload.ServiceIDIsNull != nil && *payload.ServiceIDIsNull && payload.ServiceID != nil {
		return ValidationError{Detail: "service_id and service_id_is_null cannot both be set"}
	}
	return nil
}

func validateUserBase(payload *UserPayloadBase, requireProxies bool, catalog MutationContext) error {
	if payload == nil {
		return nil
	}
	if payload.DataLimit != nil && *payload.DataLimit < 0 {
		return ValidationError{Detail: "data_limit must be greater than or equal to 0"}
	}
	if payload.DataLimitResetStrategy == "" {
		payload.DataLimitResetStrategy = UserDataLimitResetNoReset
	}
	if !validResetStrategy(payload.DataLimitResetStrategy) {
		return ValidationError{Detail: "invalid data_limit_reset_strategy"}
	}
	if payload.Note != nil {
		note := strings.TrimSpace(*payload.Note)
		if len(note) > 500 {
			return ValidationError{Detail: "User's note can be a maximum of 500 character"}
		}
		*payload.Note = note
	}
	if payload.TelegramID != nil {
		value := strings.TrimSpace(*payload.TelegramID)
		if value == "" {
			payload.TelegramID = nil
		} else if !telegramRegexp.MatchString(value) {
			return ValidationError{Detail: "Invalid telegram_id format"}
		} else {
			*payload.TelegramID = value
		}
	}
	if payload.ContactNumber != nil {
		value := strings.TrimSpace(*payload.ContactNumber)
		if value == "" {
			payload.ContactNumber = nil
		} else if !contactRegexp.MatchString(value) {
			return ValidationError{Detail: "Invalid contact_number format"}
		} else {
			*payload.ContactNumber = value
		}
	}
	if payload.Flow != nil {
		normalized, ok := NormalizeFlow(*payload.Flow)
		if !ok {
			return ValidationError{Detail: "Unsupported flow value"}
		}
		if normalized == "" {
			payload.Flow = nil
		} else {
			*payload.Flow = normalized
		}
	}
	if payload.IPLimit != nil && *payload.IPLimit < 0 {
		zero := int64(0)
		payload.IPLimit = &zero
	}
	if payload.OnHoldExpireDuration != nil && *payload.OnHoldExpireDuration <= 0 {
		payload.OnHoldExpireDuration = nil
	}
	if payload.OnHoldTimeout != nil && strings.TrimSpace(*payload.OnHoldTimeout) == "" {
		payload.OnHoldTimeout = nil
	}
	if err := validateNextPlans(payload.NextPlan, payload.NextPlans); err != nil {
		return err
	}
	if err := validateProxiesAndInbounds(payload.Proxies, payload.Inbounds, requireProxies, catalog); err != nil {
		return err
	}
	return nil
}

func validateUsername(value string) error {
	if len(value) < 3 || len(value) > 32 || !usernameRegexp.MatchString(value) {
		return ValidationError{Detail: UsernameValidationMessage}
	}
	return nil
}

func NormalizeFlow(value string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "", true
	}
	switch normalized {
	case "xtls-rprx-vision", "xtls-rprx-vision-udp443":
		return normalized, true
	default:
		return "", false
	}
}

func validateOnHoldCreate(status UserStatusCreate, duration *int64, expire *int64) error {
	if status != UserStatusCreateOnHold {
		return nil
	}
	return validateOnHold(duration, expire)
}

func validateOnHoldModify(status UserStatusModify, duration *int64, expire *int64) error {
	if status != UserStatusModifyOnHold {
		return nil
	}
	return validateOnHold(duration, expire)
}

func validateOnHold(duration *int64, expire *int64) error {
	if duration == nil || *duration <= 0 {
		return ValidationError{Detail: "User cannot be on hold without a valid on_hold_expire_duration."}
	}
	if expire != nil && *expire > 0 {
		return ValidationError{Detail: "User cannot be on hold with specified expire."}
	}
	return nil
}

func validateNextPlans(nextPlan *NextPlanPayload, nextPlans []NextPlanPayload) error {
	if nextPlan != nil {
		if err := validateNextPlan(nextPlan); err != nil {
			return err
		}
	}
	for i := range nextPlans {
		if err := validateNextPlan(&nextPlans[i]); err != nil {
			return err
		}
	}
	return nil
}

func validateNextPlan(plan *NextPlanPayload) error {
	if plan == nil {
		return nil
	}
	if plan.DataLimit != nil && *plan.DataLimit < 0 {
		return ValidationError{Detail: "next plan data_limit must be greater than or equal to 0"}
	}
	if plan.TriggerOn == "" {
		plan.TriggerOn = "either"
	}
	return nil
}

func validateProxiesAndInbounds(
	proxies ProxyPayload,
	inbounds map[string][]string,
	requireProxies bool,
	catalog MutationContext,
) error {
	if requireProxies && len(proxies) == 0 {
		return ValidationError{Detail: "Each user needs at least one proxy"}
	}
	normalizedProxies := map[string]struct{}{}
	for protocol, settings := range proxies {
		protocol = normalizeProtocol(protocol)
		if !validProxyProtocol(protocol) {
			return ValidationError{Detail: fmt.Sprintf("Unsupported proxy type %s", protocol)}
		}
		if settings == nil {
			return ValidationError{Detail: fmt.Sprintf("%s proxy settings must be an object", protocol)}
		}
		normalizedProxies[protocol] = struct{}{}
	}
	for protocol, tags := range inbounds {
		protocol = normalizeProtocol(protocol)
		if !validProxyProtocol(protocol) {
			return ValidationError{Detail: fmt.Sprintf("Unsupported proxy type %s", protocol)}
		}
		if len(normalizedProxies) > 0 {
			if _, ok := normalizedProxies[protocol]; !ok {
				return ValidationError{Detail: fmt.Sprintf("%s inbounds cannot be set without proxy settings", protocol)}
			}
		}
		for _, tag := range tags {
			info, ok := catalog.Inbounds[tag]
			if len(catalog.Inbounds) > 0 && !ok {
				return ValidationError{Detail: fmt.Sprintf("Inbound %s doesn't exist", tag)}
			}
			if ok && normalizeProtocol(info.Protocol) != protocol {
				return ValidationError{Detail: fmt.Sprintf("Inbound %s doesn't match %s protocol", tag, protocol)}
			}
			if ok && !info.HasEnabledHosts {
				return ValidationError{Detail: fmt.Sprintf("Inbound %s has no enabled hosts", tag)}
			}
		}
	}
	return nil
}

func validResetStrategy(value UserDataLimitResetStrategy) bool {
	switch value {
	case UserDataLimitResetNoReset, UserDataLimitResetDay, UserDataLimitResetWeek, UserDataLimitResetMonth, UserDataLimitResetYear:
		return true
	default:
		return false
	}
}

func validProxyProtocol(protocol string) bool {
	switch normalizeProtocol(protocol) {
	case "vmess", "vless", "trojan", "shadowsocks":
		return true
	default:
		return false
	}
}

func normalizeProtocol(protocol string) string {
	return strings.ToLower(strings.TrimSpace(protocol))
}

type AutoServiceDetection struct {
	ServiceID int64
	Tag       string
	Detected  bool
}

func DetectAutoServiceFromInbounds(inbounds map[string][]string) (AutoServiceDetection, error) {
	if len(inbounds) == 0 {
		return AutoServiceDetection{}, nil
	}
	tags := map[string]struct{}{}
	for _, values := range inbounds {
		for _, tag := range values {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tags[tag] = struct{}{}
			}
		}
	}
	if len(tags) == 0 {
		return AutoServiceDetection{}, nil
	}
	autoTags := make([]string, 0)
	for tag := range tags {
		if autoServiceRegexp.MatchString(tag) {
			autoTags = append(autoTags, tag)
		}
	}
	sort.Strings(autoTags)
	if len(autoTags) == 0 {
		return AutoServiceDetection{}, nil
	}
	if len(autoTags) > 1 {
		return AutoServiceDetection{}, ValidationError{Detail: "Only one service inbound can be selected at a time."}
	}
	if len(tags) != 1 {
		return AutoServiceDetection{}, ValidationError{Detail: "Service inbound must be selected alone without any additional inbounds."}
	}
	match := autoServiceRegexp.FindStringSubmatch(autoTags[0])
	if len(match) != 2 {
		return AutoServiceDetection{}, nil
	}
	serviceID, err := parsePositiveInt64(match[1])
	if err != nil {
		return AutoServiceDetection{}, ValidationError{Detail: fmt.Sprintf(`Invalid service inbound tag "%s".`, autoTags[0])}
	}
	return AutoServiceDetection{ServiceID: serviceID, Tag: autoTags[0], Detected: true}, nil
}

func parsePositiveInt64(value string) (int64, error) {
	var parsed int64
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid integer")
		}
		parsed = parsed*10 + int64(ch-'0')
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("invalid integer")
	}
	return parsed, nil
}
