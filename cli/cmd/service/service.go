package service

import (
	"github.com/rilldata/rill/cli/cmd/service/token"
	"github.com/rilldata/rill/cli/pkg/cmdutil"
	adminv1 "github.com/rilldata/rill/proto/gen/rill/admin/v1"
	"github.com/spf13/cobra"
)

func ServiceCmd(ch *cmdutil.Helper) *cobra.Command {
	cfg := ch.Config
	serviceCmd := &cobra.Command{
		Use:               "service",
		Short:             "Manage service accounts",
		PersistentPreRunE: cmdutil.CheckChain(cmdutil.CheckAuth(cfg), cmdutil.CheckOrganization(cfg)),
	}

	serviceCmd.PersistentFlags().StringVar(&cfg.Org, "org", cfg.Org, "Organization Name")
	serviceCmd.AddCommand(ListCmd(ch))
	serviceCmd.AddCommand(CreateCmd(ch))
	serviceCmd.AddCommand(RenameCmd(ch))
	serviceCmd.AddCommand(DeleteCmd(ch))
	serviceCmd.AddCommand(token.TokenCmd(ch))

	return serviceCmd
}

func toRow(s *adminv1.Service) *service {
	return &service{
		Name:      s.Name,
		OrgName:   s.OrgName,
		CreatedAt: s.CreatedOn.AsTime().Format(cmdutil.TSFormatLayout),
	}
}

func toTable(sv []*adminv1.Service) []*service {
	services := make([]*service, 0, len(sv))

	for _, s := range sv {
		services = append(services, toRow(s))
	}

	return services
}

type service struct {
	Name      string `header:"name" json:"name"`
	OrgName   string `header:"org_name" json:"org_name"`
	CreatedAt string `header:"created_at,timestamp(ms|utc|human)" json:"created_at"`
}
