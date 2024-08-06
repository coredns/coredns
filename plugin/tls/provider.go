package tls

import (
	"os"

	"github.com/caddyserver/certmagic"
	"github.com/libdns/alidns"
	"github.com/libdns/azure"
	"github.com/libdns/cloudflare"
	"github.com/libdns/digitalocean"
	"github.com/libdns/duckdns"
	"github.com/libdns/googleclouddns"
	"github.com/libdns/hetzner"
	"github.com/libdns/ionos"
	"github.com/libdns/linode"
	"github.com/libdns/namecheap"
	"github.com/libdns/netlify"
	"github.com/libdns/powerdns"
	"github.com/libdns/route53"
	"github.com/libdns/vercel"
)

func createDNSSolver(provider string) *certmagic.DNS01Solver {
	var dnsSolver *certmagic.DNS01Solver
	switch provider {
	case "digitalocean":
		dnsSolver = &certmagic.DNS01Solver{
			DNSManager: certmagic.DNSManager{
				DNSProvider: &digitalocean.Provider{
					APIToken: os.Getenv("DO_AUTH_TOKEN"),
				},
			},
		}
	case "cloudflare":
		dnsSolver = &certmagic.DNS01Solver{
			DNSManager: certmagic.DNSManager{
				DNSProvider: &cloudflare.Provider{
					APIToken: os.Getenv("CLOUDFLARE_AUTH_TOKEN"),
				},
			},
		}
	case "hetzner":
		dnsSolver = &certmagic.DNS01Solver{
			DNSManager: certmagic.DNSManager{
				DNSProvider: &hetzner.Provider{
					AuthAPIToken: os.Getenv("HETZNER_AUTH_TOKEN"),
				},
			},
		}
	case "netlify":
		dnsSolver = &certmagic.DNS01Solver{
			DNSManager: certmagic.DNSManager{
				DNSProvider: &netlify.Provider{
					PersonalAccessToken: os.Getenv("NETLIFY_AUTH_TOKEN"),
				},
			},
		}
	case "namecheap":
		dnsSolver = &certmagic.DNS01Solver{
			DNSManager: certmagic.DNSManager{
				DNSProvider: &namecheap.Provider{
					User:        os.Getenv("NAMECHEAP_USER"),
					ClientIP:    os.Getenv("NAMECHEAP_CLIENT_IP"),
					APIKey:      os.Getenv("NAMECHEAP_API_KEY"),
					APIEndpoint: os.Getenv("NAMECHEAP_API_ENDPOINT"),
				},
			},
		}
	case "ionos":
		dnsSolver = &certmagic.DNS01Solver{
			DNSManager: certmagic.DNSManager{
				DNSProvider: &ionos.Provider{
					AuthAPIToken: os.Getenv("IONOS_AUTH_TOKEN"),
				},
			},
		}
	case "duckdns":
		dnsSolver = &certmagic.DNS01Solver{
			DNSManager: certmagic.DNSManager{
				DNSProvider: &duckdns.Provider{
					APIToken:       os.Getenv("DUCKDNS_TOKEN"),
					OverrideDomain: os.Getenv("DUCKDNS_OVERRIDE_DOMAIN"),
				},
			},
		}
	case "route53":
		dnsSolver = &certmagic.DNS01Solver{
			DNSManager: certmagic.DNSManager{
				DNSProvider: &route53.Provider{
					Region:      os.Getenv("AWS_REGION"),
					Profile:     os.Getenv("AWS_PROFILE"),
					AccessKeyId: os.Getenv("AWS_ACCESS_KEY_ID"),
				},
			},
		}
	case "powerdns":
		dnsSolver = &certmagic.DNS01Solver{
			DNSManager: certmagic.DNSManager{
				DNSProvider: &powerdns.Provider{
					ServerURL: os.Getenv("AWS_REGION"),
					ServerID:  os.Getenv("AWS_PROFILE"),
					APIToken:  os.Getenv("AWS_ACCESS_KEY_ID"),
				},
			},
		}
	case "vercel":
		dnsSolver = &certmagic.DNS01Solver{
			DNSManager: certmagic.DNSManager{
				DNSProvider: &vercel.Provider{
					AuthAPIToken: os.Getenv("VERCEL_AUTH_TOKEN"),
				},
			},
		}
	case "azure":
		dnsSolver = &certmagic.DNS01Solver{
			DNSManager: certmagic.DNSManager{
				DNSProvider: &azure.Provider{
					SubscriptionId:    os.Getenv("AZURE_SUBSCRIPTION_ID"),
					ResourceGroupName: os.Getenv("AZURE_RESOURCE_GROUP_NAME"),
					TenantId:          os.Getenv("AZURE_TENANT_ID"),
					ClientId:          os.Getenv("AZURE_CLIENT_ID"),
				},
			},
		}
	case "alidns":
		dnsSolver = &certmagic.DNS01Solver{
			DNSManager: certmagic.DNSManager{
				DNSProvider: &alidns.Provider{
					AccKeyID:     os.Getenv("ALI_ACCOUNT_KEY"),
					AccKeySecret: os.Getenv("ALI_ACCOUNT_KEY_SECRET"),
					RegionID:     os.Getenv("ALI_REGION"),
				},
			},
		}
	case "googleclouddns":
		dnsSolver = &certmagic.DNS01Solver{
			DNSManager: certmagic.DNSManager{
				DNSProvider: &googleclouddns.Provider{
					Project:            os.Getenv("GOOGLE_CLOUD_PROJECT"),
					ServiceAccountJSON: os.Getenv("GOOGLE_CLOUD_SERVICE_ACCOUNT_JSON"),
				},
			},
		}
	case "linode":
		dnsSolver = &certmagic.DNS01Solver{
			DNSManager: certmagic.DNSManager{
				DNSProvider: &linode.Provider{
					APIToken:   os.Getenv("LINODE_API_TOKEN"),
					APIURL:     os.Getenv("LINODE_API_URL"),
					APIVersion: os.Getenv("LINODE_API_VERSION"),
				},
			},
		}
	}

	return dnsSolver
}
