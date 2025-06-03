package router
import (
	"github.com/hamzaKhattat/asterisk-router-production/internal/loadbalancer"
)
// GetLoadBalancer returns the router's load balancer instance
func (r *Router) GetLoadBalancer() *loadbalancer.LoadBalancer {
    return r.loadBalancer
}
