package handler

import (
	"context"
	"fmt"

	clientv1 "github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/gen/proto/client/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/middleware"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/repository"
	svc "github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// ClientServiceInterface allows handler tests to inject a mock service.
type ClientServiceInterface interface {
	CreateClient(input svc.CreateClientInput) (*models.Client, string, error)
	GetClient(id uint) (*models.Client, error)
	ListClients(filter repository.ClientFilter) ([]models.Client, int64, error)
	UpdateClient(id uint, input svc.UpdateClientInput) (*models.Client, error)
	UpdateClientPermissions(id uint, permissionNames []string) (*models.Client, error)
}

type ClientHandler struct {
	clientv1.UnimplementedClientServiceServer
	svc ClientServiceInterface
}

func NewClientHandler(cfg *config.Config, db *gorm.DB) *ClientHandler {
	return &ClientHandler{
		svc: svc.NewClientService(cfg, db),
	}
}

func NewClientHandlerWithService(s ClientServiceInterface) *ClientHandler {
	return &ClientHandler{svc: s}
}

func toClientProto(client *models.Client) *clientv1.ClientProto {
	perms := make([]*clientv1.PermissionProto, 0, len(client.Permissions))
	for _, p := range client.Permissions {
		perms = append(perms, &clientv1.PermissionProto{
			Id:          uint64(p.ID),
			Name:        p.Name,
			Description: p.Description,
		})
	}

	return &clientv1.ClientProto{
		Id:             uint64(client.ID),
		Ime:            client.Ime,
		Prezime:        client.Prezime,
		DatumRodjenja:  client.DatumRodjenja,
		Pol:            client.Pol,
		Email:          client.Email,
		BrojTelefona:   client.BrojTelefona,
		Adresa:         client.Adresa,
		PovezaniRacuni: client.PovezaniRacuni,
		Permissions:    perms,
	}
}

func toClientListItem(client *models.Client) *clientv1.ClientListItem {
	return &clientv1.ClientListItem{
		Id:              uint64(client.ID),
		Ime:             client.Ime,
		Prezime:         client.Prezime,
		Email:           client.Email,
		BrojTelefona:    client.BrojTelefona,
		PovezaniRacuni:  client.PovezaniRacuni,
		PermissionNames: client.PermissionNames(),
	}
}

func (h *ClientHandler) CreateClient(ctx context.Context, req *clientv1.CreateClientRequest) (*clientv1.CreateClientResponse, error) {
	client, setupToken, err := h.svc.CreateClient(svc.CreateClientInput{
		Ime:            req.Ime,
		Prezime:        req.Prezime,
		DatumRodjenja:  req.DatumRodjenja,
		Pol:            req.Pol,
		Email:          req.Email,
		BrojTelefona:   req.BrojTelefona,
		Adresa:         req.Adresa,
		PovezaniRacuni: req.PovezaniRacuni,
	})
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err.Error())
	}

	return &clientv1.CreateClientResponse{
		Client:  toClientProto(client),
		Message: fmt.Sprintf("Client created. Complete initial password setup with token: %s", setupToken),
	}, nil
}

func (h *ClientHandler) GetClient(ctx context.Context, req *clientv1.GetClientRequest) (*clientv1.GetClientResponse, error) {
	if claims, ok := middleware.GetClaimsFromContext(ctx); ok && claims.ClientID != 0 && uint(req.Id) != claims.ClientID {
		return nil, status.Error(codes.PermissionDenied, "access denied")
	}

	client, err := h.svc.GetClient(uint(req.Id))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "client not found")
	}

	return &clientv1.GetClientResponse{Client: toClientProto(client)}, nil
}

func (h *ClientHandler) ListClients(ctx context.Context, req *clientv1.ListClientsRequest) (*clientv1.ListClientsResponse, error) {
	filter := repository.ClientFilter{
		Email:    req.EmailFilter,
		Name:     req.NameFilter,
		Page:     int(req.Page),
		PageSize: int(req.PageSize),
	}

	clients, total, err := h.svc.ListClients(filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list clients")
	}

	items := make([]*clientv1.ClientListItem, 0, len(clients))
	for i := range clients {
		items = append(items, toClientListItem(&clients[i]))
	}

	return &clientv1.ListClientsResponse{
		Clients:  items,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}

func (h *ClientHandler) UpdateClient(ctx context.Context, req *clientv1.UpdateClientRequest) (*clientv1.UpdateClientResponse, error) {
	if claims, ok := middleware.GetClaimsFromContext(ctx); ok && claims.ClientID != 0 && uint(req.Id) != claims.ClientID {
		return nil, status.Error(codes.PermissionDenied, "access denied")
	}

	client, err := h.svc.UpdateClient(uint(req.Id), svc.UpdateClientInput{
		Ime:            req.Ime,
		Prezime:        req.Prezime,
		DatumRodjenja:  req.DatumRodjenja,
		Pol:            req.Pol,
		Email:          req.Email,
		BrojTelefona:   req.BrojTelefona,
		Adresa:         req.Adresa,
		PovezaniRacuni: req.PovezaniRacuni,
	})
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err.Error())
	}

	return &clientv1.UpdateClientResponse{Client: toClientProto(client)}, nil
}

func (h *ClientHandler) UpdateClientPermissions(ctx context.Context, req *clientv1.UpdateClientPermissionsRequest) (*clientv1.UpdateClientPermissionsResponse, error) {
	client, err := h.svc.UpdateClientPermissions(uint(req.Id), req.PermissionNames)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err.Error())
	}

	perms := make([]*clientv1.PermissionProto, 0, len(client.Permissions))
	for _, p := range client.Permissions {
		perms = append(perms, &clientv1.PermissionProto{
			Id:          uint64(p.ID),
			Name:        p.Name,
			Description: p.Description,
		})
	}

	return &clientv1.UpdateClientPermissionsResponse{
		Permissions: perms,
		Message:     "Client permissions updated",
	}, nil
}
