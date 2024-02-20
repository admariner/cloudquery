package compute

import (
	pb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/cloudquery/cloudquery/plugins/source/gcp/client"
	"github.com/cloudquery/plugin-sdk/v4/schema"
	"github.com/cloudquery/plugin-sdk/v4/transformers"
)

func Projects() *schema.Table {
	return &schema.Table{
		Name:        "gcp_compute_projects",
		Description: `https://cloud.google.com/compute/docs/reference/rest/v1/projects#resource:-project`,
		Resolver:    fetchProjects,
		Multiplex:   client.ProjectMultiplexEnabledServices("compute.googleapis.com"),
		Transform:   client.TransformWithStruct(&pb.Project{}, transformers.WithPrimaryKeys("SelfLink")),
		Columns: []schema.Column{
			client.ProjectIDColumn(false),
		},
	}
}
