package loadbalancer

import (
    "fmt"
    "math/rand"
    "sort"
    "sync"
    "time"
    
    "github.com/hamzaKhattat/asterisk-router-production/internal/db"
    "github.com/hamzaKhattat/asterisk-router-production/internal/models"
)

type LoadBalancer struct {
    mu              sync.RWMutex
    providerStats   map[string]*models.LoadBalancerStats
    roundRobinIndex map[string]int
}

func New() *LoadBalancer {
    return &LoadBalancer{
        providerStats:   make(map[string]*models.LoadBalancerStats),
        roundRobinIndex: make(map[string]int),
    }
}

func (lb *LoadBalancer) SelectProvider(providers []*models.Provider, mode string) (*models.Provider, error) {
    if len(providers) == 0 {
        return nil, fmt.Errorf("no providers available")
    }
    
    // Filter only active and healthy providers
    activeProviders := lb.filterHealthyProviders(providers)
    if len(activeProviders) == 0 {
        return nil, fmt.Errorf("no healthy providers available")
    }
    
    switch mode {
    case "round_robin":
        return lb.roundRobin(activeProviders)
    case "weighted":
        return lb.weightedRandom(activeProviders)
    case "priority":
        return lb.priority(activeProviders)
    case "failover":
        return lb.failover(activeProviders)
    default:
        return lb.roundRobin(activeProviders)
    }
}

func (lb *LoadBalancer) filterHealthyProviders(providers []*models.Provider) []*models.Provider {
    lb.mu.RLock()
    defer lb.mu.RUnlock()
    
    var healthy []*models.Provider
    for _, p := range providers {
        if p.Active {
            stats, exists := lb.providerStats[p.Name]
            if !exists || stats.IsHealthy {
                // Check max channels limit
                if p.MaxChannels == 0 || stats == nil || stats.ActiveCalls < int64(p.MaxChannels) {
                    healthy = append(healthy, p)
                }
            }
        }
    }
    return healthy
}

func (lb *LoadBalancer) roundRobin(providers []*models.Provider) (*models.Provider, error) {
    lb.mu.Lock()
    defer lb.mu.Unlock()
    
    key := fmt.Sprintf("%v", providers)
    index := lb.roundRobinIndex[key]
    provider := providers[index%len(providers)]
    lb.roundRobinIndex[key] = index + 1
    
    return provider, nil
}

func (lb *LoadBalancer) weightedRandom(providers []*models.Provider) (*models.Provider, error) {
    totalWeight := 0
    for _, p := range providers {
        totalWeight += p.Weight
    }
    
    if totalWeight == 0 {
        return providers[rand.Intn(len(providers))], nil
    }
    
    r := rand.Intn(totalWeight)
    for _, p := range providers {
        r -= p.Weight
        if r < 0 {
            return p, nil
        }
    }
    
    return providers[len(providers)-1], nil
}

func (lb *LoadBalancer) priority(providers []*models.Provider) (*models.Provider, error) {
    sort.Slice(providers, func(i, j int) bool {
        return providers[i].Priority > providers[j].Priority
    })
    
    return providers[0], nil
}

func (lb *LoadBalancer) failover(providers []*models.Provider) (*models.Provider, error) {
    sort.Slice(providers, func(i, j int) bool {
        return providers[i].Priority > providers[j].Priority
    })
    
    for _, p := range providers {
        stats := lb.GetProviderStats(p.Name)
        if stats.IsHealthy {
            return p, nil
        }
    }
    
    // If all unhealthy, return highest priority
    return providers[0], nil
}

func (lb *LoadBalancer) UpdateStats(providerName string, callSucceeded bool, duration time.Duration) {
    lb.mu.Lock()
    defer lb.mu.Unlock()
    
    stats, exists := lb.providerStats[providerName]
    if !exists {
        stats = &models.LoadBalancerStats{
            ProviderName: providerName,
            IsHealthy:    true,
        }
        lb.providerStats[providerName] = stats
    }
    
    stats.TotalCalls++
    stats.LastCallTime = time.Now()
    
    if callSucceeded {
        if duration > 0 {
            stats.AvgCallDuration = (stats.AvgCallDuration*float64(stats.TotalCalls-1) + duration.Seconds()) / float64(stats.TotalCalls)
        }
    } else {
        stats.FailedCalls++
    }
    
    stats.SuccessRate = float64(stats.TotalCalls-stats.FailedCalls) / float64(stats.TotalCalls) * 100
    
    // Mark unhealthy if success rate drops below 50%
    if stats.TotalCalls > 10 && stats.SuccessRate < 50 {
        stats.IsHealthy = false
    }
    
    // Update database
    go lb.updateStatsInDB(stats)
}

func (lb *LoadBalancer) IncrementActiveCalls(providerName string, delta int64) {
    lb.mu.Lock()
    defer lb.mu.Unlock()
    
    stats, exists := lb.providerStats[providerName]
    if !exists {
        stats = &models.LoadBalancerStats{
            ProviderName: providerName,
            IsHealthy:    true,
        }
        lb.providerStats[providerName] = stats
    }
    
    stats.ActiveCalls += delta
    if stats.ActiveCalls < 0 {
        stats.ActiveCalls = 0
    }
}

func (lb *LoadBalancer) GetProviderStats(providerName string) models.LoadBalancerStats {
    lb.mu.RLock()
    defer lb.mu.RUnlock()
    
    if stats, exists := lb.providerStats[providerName]; exists {
        return *stats
    }
    
    return models.LoadBalancerStats{
        ProviderName: providerName,
        IsHealthy:    true,
    }
}

func (lb *LoadBalancer) updateStatsInDB(stats *models.LoadBalancerStats) {
    query := `
        INSERT INTO provider_stats (provider_name, total_calls, active_calls, failed_calls, 
                                   success_rate, avg_call_duration, last_call_time, is_healthy)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
        ON DUPLICATE KEY UPDATE
            total_calls = VALUES(total_calls),
            active_calls = VALUES(active_calls),
            failed_calls = VALUES(failed_calls),
            success_rate = VALUES(success_rate),
            avg_call_duration = VALUES(avg_call_duration),
            last_call_time = VALUES(last_call_time),
            is_healthy = VALUES(is_healthy)`
    
    db.DB.Exec(query, stats.ProviderName, stats.TotalCalls, stats.ActiveCalls,
        stats.FailedCalls, stats.SuccessRate, stats.AvgCallDuration,
        stats.LastCallTime, stats.IsHealthy)
}

func (lb *LoadBalancer) StartHealthMonitor() {
    go func() {
        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()
        
        for range ticker.C {
            lb.checkProviderHealth()
        }
    }()
}

func (lb *LoadBalancer) checkProviderHealth() {
    lb.mu.Lock()
    defer lb.mu.Unlock()
    
    for name, stats := range lb.providerStats {
        // Reset health if no calls in last 5 minutes
        fmt.Println(name)
        if time.Since(stats.LastCallTime) > 5*time.Minute && !stats.IsHealthy {
            stats.IsHealthy = true
            stats.FailedCalls = 0
            stats.TotalCalls = 0
            stats.SuccessRate = 100
        }
    }
}
