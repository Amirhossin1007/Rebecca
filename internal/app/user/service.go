package user

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	adminapp "github.com/rebeccapanel/rebecca/internal/app/admin"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return Service{repo: repo}
}

func (s Service) LinkPrerequisites(ctx context.Context, req LinkPrerequisitesRequest) (LinkPrerequisites, error) {
	if len(req.UserIDs) == 0 && len(req.ServiceIDs) == 0 && len(req.AdminIDs) == 0 {
		return LinkPrerequisites{}, fmt.Errorf("at least one user_id, service_id, or admin_id is required")
	}
	return s.repo.LinkPrerequisites(ctx, req)
}

func (s Service) SubscriptionLinks(ctx context.Context, req SubscriptionLinkRequest) (SubscriptionLinks, error) {
	if req.Username == "" {
		return SubscriptionLinks{}, fmt.Errorf("username is required")
	}
	settings, err := s.repo.subscriptionSettings(ctx)
	if err != nil {
		return SubscriptionLinks{}, err
	}
	secret, err := s.repo.subscriptionSecretKey(ctx)
	if err != nil {
		return SubscriptionLinks{}, err
	}
	admin := AdminLinkSettings{}
	if req.AdminID != nil && *req.AdminID > 0 {
		admins, err := s.repo.adminLinkSettings(ctx, []int64{*req.AdminID})
		if err != nil {
			return SubscriptionLinks{}, err
		}
		admin = admins[*req.AdminID]
	}
	return BuildSubscriptionLinks(req, settings, admin, secret)
}

func (s Service) ConfigLinks(ctx context.Context, req ConfigLinksRequest) (ConfigLinksResponse, error) {
	item := ConfigLinkUser{}
	if req.User != nil {
		item = *req.User
	} else {
		loaded, err := s.repo.ConfigLinkUser(ctx, req.UserID)
		if err != nil {
			return ConfigLinksResponse{}, err
		}
		item = loaded
	}
	if item.Username == "" {
		return ConfigLinksResponse{}, fmt.Errorf("username is required")
	}

	inboundOrder := item.XrayInboundOrder
	if len(item.XrayInboundsByTag) == 0 {
		inbounds, order, err := s.repo.ResolvedInboundsByTag(ctx)
		if err != nil {
			return ConfigLinksResponse{}, err
		}
		item.XrayInboundsByTag = inbounds
		inboundOrder = order
	}
	if len(item.Hosts) == 0 {
		hosts, err := s.repo.hosts(ctx)
		if err != nil {
			return ConfigLinksResponse{}, err
		}
		item.Hosts = hosts
	}
	if item.ServiceID != nil && item.ServiceHostOrders == nil {
		orders, err := s.repo.serviceHostOrders(ctx, []int64{*item.ServiceID})
		if err != nil {
			return ConfigLinksResponse{}, err
		}
		item.ServiceHostOrders = orders[*item.ServiceID]
	}
	masks, err := s.repo.uuidMasks(ctx)
	if err != nil {
		return ConfigLinksResponse{}, err
	}
	return BuildConfigLinks(item, item.XrayInboundsByTag, inboundOrder, item.Hosts, masks, req.Reverse)
}

func (s Service) UsersList(ctx context.Context, req UsersListRequest) (UsersResponse, error) {
	return s.repo.UsersList(ctx, req)
}

func (s Service) UserGet(ctx context.Context, req UserGetRequest) (UserDetail, error) {
	if strings.TrimSpace(req.Username) == "" {
		return UserDetail{}, fmt.Errorf("username is required")
	}
	return s.repo.UserGet(ctx, req)
}

func (s Service) CreateUser(ctx context.Context, admin adminapp.Admin, raw []byte) (MutationResult, error) {
	fields, err := decodeRawFields(raw)
	if err != nil {
		return MutationResult{}, clientError(400, "invalid request body")
	}
	var serviceID *int64
	if rawFieldPresent(fields, "service_id") && !rawIsNull(fields["service_id"]) {
		var parsed int64
		if err := json.Unmarshal(fields["service_id"], &parsed); err != nil {
			return MutationResult{}, clientError(400, "invalid service_id")
		}
		serviceID = &parsed
	}
	var payload UserCreate
	if err := json.Unmarshal(raw, &payload); err != nil {
		return MutationResult{}, clientError(400, "invalid request body")
	}
	if auto, err := DetectAutoServiceFromInbounds(payload.Inbounds); err != nil {
		return MutationResult{}, clientError(400, err.Error())
	} else if serviceID == nil && auto.Detected {
		serviceID = &auto.ServiceID
	}
	return s.repo.createUserMutation(ctx, admin, payload, serviceID)
}

func (s Service) UpdateUser(ctx context.Context, admin adminapp.Admin, username string, raw []byte) (MutationResult, error) {
	fields, err := decodeRawFields(raw)
	if err != nil {
		return MutationResult{}, clientError(400, "invalid request body")
	}
	var payload UserModify
	if err := json.Unmarshal(raw, &payload); err != nil {
		return MutationResult{}, clientError(400, "invalid request body")
	}
	if rawFieldPresent(fields, "service_id") && rawIsNull(fields["service_id"]) {
		payload.ServiceID = nil
	}
	if auto, err := DetectAutoServiceFromInbounds(payload.Inbounds); err != nil {
		return MutationResult{}, clientError(400, err.Error())
	} else if auto.Detected && !rawFieldPresent(fields, "service_id") {
		payload.ServiceID = &auto.ServiceID
		fields["service_id"] = []byte(fmt.Sprintf("%d", auto.ServiceID))
	}
	return s.repo.updateUserMutation(ctx, admin, username, payload, fields)
}

func (s Service) DeleteUser(ctx context.Context, admin adminapp.Admin, username string) (MutationResult, error) {
	return s.repo.deleteUserMutation(ctx, admin, username)
}

func (s Service) ResetUser(ctx context.Context, admin adminapp.Admin, username string) (MutationResult, error) {
	return s.repo.resetUserMutation(ctx, admin, username)
}

func (s Service) RevokeUserSubscription(ctx context.Context, admin adminapp.Admin, username string) (MutationResult, error) {
	return s.repo.revokeUserMutation(ctx, admin, username)
}

func (s Service) ActiveNextPlan(ctx context.Context, admin adminapp.Admin, username string) (MutationResult, error) {
	return s.repo.activeNextMutation(ctx, admin, username)
}

func (s Service) BulkUsersAction(ctx context.Context, admin adminapp.Admin, payload BulkUsersActionRequest, opts BulkUsersActionOptions) (BulkUsersActionResult, error) {
	if err := ValidateBulkUsersAction(&payload); err != nil {
		return BulkUsersActionResult{}, clientError(400, err.Error())
	}
	return s.repo.bulkUsersActionMutation(ctx, admin, payload, opts)
}

func decodeRawFields(raw []byte) (map[string]json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return fields, nil
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	return fields, nil
}

func rawIsNull(raw json.RawMessage) bool {
	return strings.EqualFold(strings.TrimSpace(string(raw)), "null")
}
