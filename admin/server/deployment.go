package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rilldata/rill/admin/database"
	"github.com/rilldata/rill/admin/pkg/urlutil"
	"github.com/rilldata/rill/admin/server/auth"
	adminv1 "github.com/rilldata/rill/proto/gen/rill/admin/v1"
	"github.com/rilldata/rill/runtime/pkg/observability"
	runtimeauth "github.com/rilldata/rill/runtime/server/auth"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) TriggerReconcile(ctx context.Context, req *adminv1.TriggerReconcileRequest) (*adminv1.TriggerReconcileResponse, error) {
	observability.AddRequestAttributes(ctx,
		attribute.String("args.deployment_id", req.DeploymentId),
	)

	depl, err := s.admin.DB.FindDeployment(ctx, req.DeploymentId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	proj, err := s.admin.DB.FindProject(ctx, depl.ProjectID)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	claims := auth.GetClaims(ctx)
	if !claims.ProjectPermissions(ctx, proj.OrganizationID, depl.ProjectID).ManageProd {
		return nil, status.Error(codes.PermissionDenied, "does not have permission to manage deployment")
	}

	err = s.admin.TriggerReconcile(ctx, depl)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	return &adminv1.TriggerReconcileResponse{}, nil
}

func (s *Server) TriggerRefreshSources(ctx context.Context, req *adminv1.TriggerRefreshSourcesRequest) (*adminv1.TriggerRefreshSourcesResponse, error) {
	observability.AddRequestAttributes(ctx,
		attribute.String("args.deployment_id", req.DeploymentId),
		attribute.StringSlice("args.sources", req.Sources),
	)

	depl, err := s.admin.DB.FindDeployment(ctx, req.DeploymentId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	proj, err := s.admin.DB.FindProject(ctx, depl.ProjectID)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	claims := auth.GetClaims(ctx)
	if !claims.ProjectPermissions(ctx, proj.OrganizationID, depl.ProjectID).ManageProd {
		return nil, status.Error(codes.PermissionDenied, "does not have permission to manage deployment")
	}

	err = s.admin.TriggerRefreshSources(ctx, depl, req.Sources)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	return &adminv1.TriggerRefreshSourcesResponse{}, nil
}

func (s *Server) triggerRefreshSourcesInternal(w http.ResponseWriter, r *http.Request) {
	orgName := r.URL.Query().Get("organization")
	projectName := r.URL.Query().Get("project")
	if orgName == "" || projectName == "" {
		http.Error(w, "organization or project not specified", http.StatusBadRequest)
		return
	}

	proj, err := s.admin.DB.FindProjectByName(r.Context(), orgName, projectName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if proj.ProdDeploymentID == nil {
		http.Error(w, "project does not have a deployment", http.StatusBadRequest)
		return
	}

	depl, err := s.admin.DB.FindDeployment(r.Context(), *proj.ProdDeploymentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = s.admin.TriggerRefreshSources(r.Context(), depl, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
}

func (s *Server) TriggerRedeploy(ctx context.Context, req *adminv1.TriggerRedeployRequest) (*adminv1.TriggerRedeployResponse, error) {
	observability.AddRequestAttributes(ctx,
		attribute.String("args.organization", req.Organization),
		attribute.String("args.project", req.Project),
		attribute.String("args.deployment_id", req.DeploymentId),
	)

	// For backwards compatibility, this RPC supports passing either DeploymentId or Organization+Project names
	var proj *database.Project
	var depl *database.Deployment
	if req.DeploymentId != "" {
		var err error
		depl, err = s.admin.DB.FindDeployment(ctx, req.DeploymentId)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		proj, err = s.admin.DB.FindProject(ctx, depl.ProjectID)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	} else {
		var err error
		proj, err = s.admin.DB.FindProjectByName(ctx, req.Organization, req.Project)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		if proj.ProdDeploymentID != nil {
			depl, err = s.admin.DB.FindDeployment(ctx, *proj.ProdDeploymentID)
			if err != nil {
				return nil, status.Error(codes.InvalidArgument, err.Error())
			}
		}
	}

	claims := auth.GetClaims(ctx)
	if !claims.ProjectPermissions(ctx, proj.OrganizationID, proj.ID).ManageProd {
		return nil, status.Error(codes.PermissionDenied, "does not have permission to manage deployment")
	}

	_, err := s.admin.TriggerRedeploy(ctx, proj, depl)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	return &adminv1.TriggerRedeployResponse{}, nil
}

// GetDeploymentCredentials returns runtime info and JWT on behalf of a specific user, or alternatively for a raw set of JWT attributes
func (s *Server) GetDeploymentCredentials(ctx context.Context, req *adminv1.GetDeploymentCredentialsRequest) (*adminv1.GetDeploymentCredentialsResponse, error) {
	observability.AddRequestAttributes(ctx,
		attribute.String("args.organization", req.Organization),
		attribute.String("args.project", req.Project),
		attribute.String("args.branch", req.Branch),
		attribute.String("args.ttl_seconds", strconv.FormatUint(uint64(req.TtlSeconds), 10)),
	)

	proj, err := s.admin.DB.FindProjectByName(ctx, req.Organization, req.Project)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if proj.ProdDeploymentID == nil {
		return nil, status.Error(codes.InvalidArgument, "project does not have a deployment")
	}

	prodDepl, err := s.admin.DB.FindDeployment(ctx, *proj.ProdDeploymentID)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if req.Branch != "" && req.Branch != prodDepl.Branch {
		return nil, status.Error(codes.InvalidArgument, "project does not have a deployment for given branch")
	}

	claims := auth.GetClaims(ctx)
	permissions := claims.ProjectPermissions(ctx, proj.OrganizationID, proj.ID)

	// If the user is not a superuser, they must have ManageProd permissions
	if !permissions.ManageProd && !claims.Superuser(ctx) {
		return nil, status.Error(codes.PermissionDenied, "does not have permission to manage deployment")
	}

	var attr map[string]any
	if req.For != nil {
		switch forVal := req.For.(type) {
		case *adminv1.GetDeploymentCredentialsRequest_UserId:
			attr, err = s.getAttributesFor(ctx, forVal.UserId, proj.OrganizationID, proj.ID)
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		case *adminv1.GetDeploymentCredentialsRequest_UserEmail:
			user, err := s.admin.DB.FindUserByEmail(ctx, forVal.UserEmail)
			// if email is not found in the database, we assume it is a non-admin user
			if errors.Is(err, database.ErrNotFound) {
				attr = map[string]any{
					"email":  forVal.UserEmail,
					"domain": forVal.UserEmail[strings.LastIndex(forVal.UserEmail, "@")+1:],
					"admin":  false,
				}
				break
			}
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			attr, err = s.getAttributesFor(ctx, user.ID, proj.OrganizationID, proj.ID)
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		case *adminv1.GetDeploymentCredentialsRequest_Attributes:
			attr = forVal.Attributes.AsMap()
		default:
			return nil, status.Error(codes.InvalidArgument, "invalid 'for' type")
		}
	}
	// if no attributes found, we add standard non-admin user attrs to ensure security policies are applied correctly
	if len(attr) == 0 {
		attr = map[string]any{
			"email":  "",
			"domain": "",
			"admin":  false,
		}
	}

	ttlDuration := time.Hour
	if req.TtlSeconds > 0 {
		ttlDuration = time.Duration(req.TtlSeconds) * time.Second
	}

	// Generate JWT
	jwt, err := s.issuer.NewToken(runtimeauth.TokenOptions{
		AudienceURL: prodDepl.RuntimeAudience,
		Subject:     claims.OwnerID(),
		TTL:         ttlDuration,
		InstancePermissions: map[string][]runtimeauth.Permission{
			prodDepl.RuntimeInstanceID: {
				// TODO: Remove ReadProfiling and ReadRepo (may require frontend changes)
				runtimeauth.ReadObjects,
				runtimeauth.ReadMetrics,
				runtimeauth.ReadProfiling,
				runtimeauth.ReadRepo,
			},
		},
		Attributes: attr,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not issue jwt: %s", err.Error())
	}

	s.admin.Used.Deployment(prodDepl.ID)

	return &adminv1.GetDeploymentCredentialsResponse{
		RuntimeHost: prodDepl.RuntimeHost,
		InstanceId:  prodDepl.RuntimeInstanceID,
		AccessToken: jwt,
		TtlSeconds:  uint32(ttlDuration.Seconds()),
	}, nil
}

func (s *Server) GetIFrame(ctx context.Context, req *adminv1.GetIFrameRequest) (*adminv1.GetIFrameResponse, error) {
	observability.AddRequestAttributes(ctx,
		attribute.String("args.organization", req.Organization),
		attribute.String("args.project", req.Project),
		attribute.String("args.branch", req.Branch),
		attribute.String("args.kind", req.Kind),
		attribute.String("args.resource", req.Resource),
		attribute.String("args.ttl_seconds", strconv.FormatUint(uint64(req.TtlSeconds), 10)),
		attribute.String("args.state", req.State),
	)

	if req.Resource == "" {
		return nil, status.Error(codes.InvalidArgument, "resource must be specified")
	}

	proj, err := s.admin.DB.FindProjectByName(ctx, req.Organization, req.Project)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if proj.ProdDeploymentID == nil {
		return nil, status.Error(codes.InvalidArgument, "project does not have a deployment")
	}

	prodDepl, err := s.admin.DB.FindDeployment(ctx, *proj.ProdDeploymentID)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if req.Branch != "" && req.Branch != prodDepl.Branch {
		return nil, status.Error(codes.InvalidArgument, "project does not have a deployment for given branch")
	}

	claims := auth.GetClaims(ctx)
	permissions := claims.ProjectPermissions(ctx, proj.OrganizationID, proj.ID)

	// If the user is not a superuser, they must have ManageProd permissions
	if !permissions.ManageProd && !claims.Superuser(ctx) {
		return nil, status.Error(codes.PermissionDenied, "does not have permission to manage deployment")
	}

	var attr map[string]any
	if req.For != nil {
		switch forVal := req.For.(type) {
		case *adminv1.GetIFrameRequest_UserId:
			attr, err = s.getAttributesFor(ctx, forVal.UserId, proj.OrganizationID, proj.ID)
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		case *adminv1.GetIFrameRequest_UserEmail:
			user, err := s.admin.DB.FindUserByEmail(ctx, forVal.UserEmail)
			// if email is not found in the database, we assume it is a non-admin user
			if errors.Is(err, database.ErrNotFound) {
				attr = map[string]any{
					"email":  forVal.UserEmail,
					"domain": forVal.UserEmail[strings.LastIndex(forVal.UserEmail, "@")+1:],
					"admin":  false,
				}
				break
			}
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			attr, err = s.getAttributesFor(ctx, user.ID, proj.OrganizationID, proj.ID)
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		case *adminv1.GetIFrameRequest_Attributes:
			attr = forVal.Attributes.AsMap()
		default:
			return nil, status.Error(codes.InvalidArgument, "invalid 'for' type")
		}
	}
	// if no attributes found, we add standard non-admin user attrs to ensure security policies are applied correctly
	if len(attr) == 0 {
		attr = map[string]any{
			"email":  "",
			"domain": "",
			"admin":  false,
		}
	}

	// default here is higher than GetDeploymentCredentials as most embedders probably won't implement refresh and state management
	ttlDuration := 24 * time.Hour
	if req.TtlSeconds > 0 {
		ttlDuration = time.Duration(req.TtlSeconds) * time.Second
	}

	// Generate JWT
	jwt, err := s.issuer.NewToken(runtimeauth.TokenOptions{
		AudienceURL: prodDepl.RuntimeAudience,
		Subject:     claims.OwnerID(),
		TTL:         ttlDuration,
		InstancePermissions: map[string][]runtimeauth.Permission{
			prodDepl.RuntimeInstanceID: {
				// TODO: Remove ReadProfiling and ReadRepo (may require frontend changes)
				runtimeauth.ReadObjects,
				runtimeauth.ReadMetrics,
				runtimeauth.ReadProfiling,
				runtimeauth.ReadRepo,
			},
		},
		Attributes: attr,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not issue jwt: %s", err.Error())
	}

	s.admin.Used.Deployment(prodDepl.ID)

	if req.Kind == "" {
		req.Kind = "MetricsView"
	}

	iFrameURL, err := urlutil.WithQuery(urlutil.MustJoinURL(s.opts.FrontendURL, "/-/embed"), map[string]string{
		"runtime_host": prodDepl.RuntimeHost,
		"instance_id":  prodDepl.RuntimeInstanceID,
		"access_token": jwt,
		"kind":         req.Kind,
		"resource":     req.Resource,
		"state":        "",
		"theme":        req.Query["theme"],
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not construct iframe url: %s", err.Error())
	}

	return &adminv1.GetIFrameResponse{
		IframeSrc:   iFrameURL,
		RuntimeHost: prodDepl.RuntimeHost,
		InstanceId:  prodDepl.RuntimeInstanceID,
		AccessToken: jwt,
		TtlSeconds:  uint32(ttlDuration.Seconds()),
	}, nil
}

func (s *Server) getAttributesFor(ctx context.Context, userID, orgID, projID string) (map[string]any, error) {
	forOrgPerms, err := s.admin.OrganizationPermissionsForUser(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}

	forProjPerms, err := s.admin.ProjectPermissionsForUser(ctx, projID, userID, forOrgPerms)
	if err != nil {
		return nil, err
	}

	attr, err := s.jwtAttributesForUser(ctx, userID, orgID, forProjPerms)
	if err != nil {
		return nil, err
	}
	return attr, nil
}
