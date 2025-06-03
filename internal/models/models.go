package models

import (
    "time"
)

// Provider represents an external server (S1, S3, S4 in UML)
type Provider struct {
    ID          int       `json:"id"`
    Name        string    `json:"name"`
    Type        string    `json:"type"` // "inbound", "intermediate", "final"
    Host        string    `json:"host"`
    Port        int       `json:"port"`
    Username    string    `json:"username"`    // Can be empty for IP-only auth
    Password    string    `json:"password"`    // Can be empty for IP-only auth
    AuthType    string    `json:"auth_type"`   // "ip", "credentials", "both"
    Codecs      []string  `json:"codecs"`
    MaxChannels int       `json:"max_channels"`
    Priority    int       `json:"priority"`
    Weight      int       `json:"weight"`
    Active      bool      `json:"active"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

// DID represents a phone number managed by S2
type DID struct {
    ID           int       `json:"id"`
    Number       string    `json:"number"`
    ProviderID   int       `json:"provider_id"`
    ProviderName string    `json:"provider_name"`
    InUse        bool      `json:"in_use"`
    Destination  string    `json:"destination"`
    Country      string    `json:"country"`
    City         string    `json:"city"`
    CreatedAt    time.Time `json:"created_at"`
    UpdatedAt    time.Time `json:"updated_at"`
}

// ProviderRoute defines complete routing path with load balancing
type ProviderRoute struct {
    ID                   int       `json:"id"`
    Name                 string    `json:"name"`
    InboundProvider      string    `json:"inbound_provider"`      // S1 in UML
    IntermediateProvider string    `json:"intermediate_provider"` // S3 in UML
    FinalProvider        string    `json:"final_provider"`        // S4 in UML
    LoadBalanceMode      string    `json:"load_balance_mode"`     // round_robin, weighted, priority, failover
    Priority             int       `json:"priority"`
    Active               bool      `json:"active"`
    CreatedAt            time.Time `json:"created_at"`
}

// CallRecord represents complete call flow through the system
type CallRecord struct {
    ID                   int64
    CallID               string
    // Original values from S1
    OriginalANI          string    // ANI-1
    OriginalDNIS         string    // DNIS-1
    // Transformed values for S3
    TransformedANI       string    // ANI-2 (=DNIS-1)
    AssignedDID          string    // DID
    // Providers involved
    InboundProvider      string    // S1
    IntermediateProvider string    // S3
    FinalProvider        string    // S4
    // Call state
    Status               string
    CurrentStep          string    // "S1_TO_S2", "S2_TO_S3", "S3_TO_S2", "S2_TO_S4", "S4_TO_S2"
    StartTime            time.Time
    EndTime              *time.Time
    Duration             int
    RecordingPath        string
}

// LoadBalancerStats tracks provider performance
type LoadBalancerStats struct {
    ProviderName    string
    TotalCalls      int64
    ActiveCalls     int64
    FailedCalls     int64
    SuccessRate     float64
    AvgCallDuration float64
    LastCallTime    time.Time
    IsHealthy       bool
}

// CallResponse for API/AGI
type CallResponse struct {
    Status      string `json:"status"`
    DIDAssigned string `json:"did_assigned"`
    NextHop     string `json:"next_hop"`
    ANIToSend   string `json:"ani_to_send"`
    DNISToSend  string `json:"dnis_to_send"`
}
