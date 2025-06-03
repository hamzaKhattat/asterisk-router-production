package cli

import (
    "bufio"
    "database/sql"
    "fmt"
    "os"
    "strconv"
    "strings"
    "time"
    
    "github.com/spf13/cobra"
    "github.com/olekukonko/tablewriter"
    "github.com/fatih/color"
    
    "github.com/hamzaKhattat/asterisk-router-production/internal/models"
    "github.com/hamzaKhattat/asterisk-router-production/internal/provider"
    "github.com/hamzaKhattat/asterisk-router-production/internal/db"
)

var providerMgr *provider.Manager

func InitCLI(pm *provider.Manager) *cobra.Command {
    providerMgr = pm
    
    rootCmd := &cobra.Command{
        Use:   "router",
        Short: "Asterisk Router Management",
        Long:  `Asterisk Router Management System
        
Manage providers, DIDs, and routes for the Asterisk routing system.`,
    }
    
    // Provider commands
    providerCmd := &cobra.Command{
        Use:   "provider",
        Short: "Manage providers",
    }
    
    providerAddCmd := &cobra.Command{
        Use:   "add <name>",
        Short: "Add a new provider",
        Args:  cobra.ExactArgs(1),
        Run:   addProvider,
    }
    
    // Provider add flags
    providerAddCmd.Flags().StringP("type", "t", "", "Provider type: inbound, intermediate, final (required)")
    providerAddCmd.Flags().StringP("host", "H", "", "Provider host/IP (required)")
    providerAddCmd.Flags().IntP("port", "p", 5060, "Provider port")
    providerAddCmd.Flags().StringP("username", "u", "", "Provider username")
    providerAddCmd.Flags().StringP("password", "P", "", "Provider password")
    providerAddCmd.Flags().StringP("codecs", "c", "ulaw,alaw", "Codecs (comma-separated)")
    providerAddCmd.Flags().IntP("max-channels", "m", 0, "Max concurrent channels (0=unlimited)")
    providerAddCmd.Flags().IntP("priority", "r", 0, "Provider priority")
    providerAddCmd.Flags().IntP("weight", "w", 1, "Provider weight for load balancing")
    
    providerAddCmd.MarkFlagRequired("type")
    providerAddCmd.MarkFlagRequired("host")
    
    providerListCmd := &cobra.Command{
        Use:   "list",
        Short: "List all providers",
        Run:   listProviders,
    }
    providerListCmd.Flags().StringP("type", "t", "", "Filter by type")
    
    providerDeleteCmd := &cobra.Command{
        Use:   "delete <name>",
        Short: "Delete a provider",
        Args:  cobra.ExactArgs(1),
        Run:   deleteProvider,
    }
    
    providerShowCmd := &cobra.Command{
        Use:   "show <name>",
        Short: "Show provider details",
        Args:  cobra.ExactArgs(1),
        Run:   showProvider,
    }
    
    providerCmd.AddCommand(providerAddCmd, providerListCmd, providerDeleteCmd, providerShowCmd)
    
    // DID commands
    didCmd := &cobra.Command{
        Use:   "did",
        Short: "Manage DIDs",
    }
    
    didAddCmd := &cobra.Command{
        Use:   "add <numbers...>",
        Short: "Add DIDs",
        Long:  "Add one or more DIDs. Use --file to import from CSV.",
        Args:  cobra.MinimumNArgs(0),
        Run:   addDIDs,
    }
    
    didAddCmd.Flags().StringP("provider", "p", "", "Provider name (required)")
    didAddCmd.Flags().StringP("country", "c", "", "Country")
    didAddCmd.Flags().StringP("city", "C", "", "City")
    didAddCmd.Flags().StringP("file", "f", "", "Import from CSV file")
    
    didListCmd := &cobra.Command{
        Use:   "list",
        Short: "List DIDs",
        Run:   listDIDs,
    }
    
    didListCmd.Flags().BoolP("all", "a", false, "Show all DIDs")
    didListCmd.Flags().StringP("provider", "p", "", "Filter by provider")
    didListCmd.Flags().Bool("in-use", false, "Show only in-use DIDs")
    didListCmd.Flags().Bool("available", false, "Show only available DIDs")
    
    didDeleteCmd := &cobra.Command{
        Use:   "delete <number>",
        Short: "Delete a DID",
        Args:  cobra.ExactArgs(1),
        Run:   deleteDID,
    }
    
    didReleaseCmd := &cobra.Command{
        Use:   "release <number>",
        Short: "Release a DID (mark as available)",
        Args:  cobra.ExactArgs(1),
        Run:   releaseDID,
    }
    
    didCmd.AddCommand(didAddCmd, didListCmd, didDeleteCmd, didReleaseCmd)
    
    // Route commands
    routeCmd := &cobra.Command{
        Use:   "route",
        Short: "Manage routes",
    }
    
    routeAddCmd := &cobra.Command{
        Use:   "add <name> <inbound> <intermediate> <final>",
        Short: "Add a new route",
        Args:  cobra.ExactArgs(4),
        Run:   addRoute,
    }
    
    routeAddCmd.Flags().StringP("mode", "m", "round_robin", "Load balance mode: round_robin, weighted, priority, failover")
    routeAddCmd.Flags().IntP("priority", "p", 0, "Route priority")
    
    routeListCmd := &cobra.Command{
        Use:   "list",
        Short: "List all routes",
        Run:   listRoutes,
    }
    
    routeDeleteCmd := &cobra.Command{
        Use:   "delete <name>",
        Short: "Delete a route",
        Args:  cobra.ExactArgs(1),
        Run:   deleteRoute,
    }
    
    routeShowCmd := &cobra.Command{
        Use:   "show <name>",
        Short: "Show route details",
        Args:  cobra.ExactArgs(1),
        Run:   showRoute,
    }
    
    routeCmd.AddCommand(routeAddCmd, routeListCmd, routeDeleteCmd, routeShowCmd)
    
    // Stats commands
    statsCmd := &cobra.Command{
        Use:   "stats",
        Short: "Show system statistics",
        Run:   showStats,
    }
    
    statsCmd.Flags().BoolP("providers", "p", false, "Show provider statistics")
    statsCmd.Flags().BoolP("calls", "c", false, "Show call statistics")
    statsCmd.Flags().BoolP("dids", "d", false, "Show DID statistics")
    
    // Load balancer command
    lbCmd := &cobra.Command{
        Use:   "lb",
        Short: "Show load balancer status",
        Run:   showLoadBalancer,
    }
    
    // Calls command
    callsCmd := &cobra.Command{
        Use:   "calls",
        Short: "Show active calls",
        Run:   showCalls,
    }
    
    callsCmd.Flags().IntP("limit", "l", 20, "Number of records to show")
    callsCmd.Flags().StringP("status", "s", "", "Filter by status")
    
    // Monitor command
    monitorCmd := &cobra.Command{
        Use:   "monitor",
        Short: "Monitor system in real-time",
        Run:   monitorSystem,
    }
    
    rootCmd.AddCommand(providerCmd, didCmd, routeCmd, statsCmd, lbCmd, callsCmd, monitorCmd)
    
    return rootCmd
}

// Provider command handlers
func addProvider(cmd *cobra.Command, args []string) {
    name := args[0]
    
    providerType, _ := cmd.Flags().GetString("type")
    host, _ := cmd.Flags().GetString("host")
    port, _ := cmd.Flags().GetInt("port")
    username, _ := cmd.Flags().GetString("username")
    password, _ := cmd.Flags().GetString("password")
    codecsStr, _ := cmd.Flags().GetString("codecs")
    maxChannels, _ := cmd.Flags().GetInt("max-channels")
    priority, _ := cmd.Flags().GetInt("priority")
    weight, _ := cmd.Flags().GetInt("weight")
    
    // Validate provider type
    validTypes := []string{"inbound", "intermediate", "final"}
    isValidType := false
    for _, t := range validTypes {
        if providerType == t {
            isValidType = true
            break
        }
    }
    
    if !isValidType {
        color.Red("Error: Invalid provider type. Must be: inbound, intermediate, or final")
        os.Exit(1)
    }
    
    // Parse codecs
    codecs := strings.Split(codecsStr, ",")
    for i := range codecs {
        codecs[i] = strings.TrimSpace(codecs[i])
    }
    
    provider := &models.Provider{
        Name:        name,
        Type:        providerType,
        Host:        host,
        Port:        port,
        Username:    username,
        Password:    password,
        Codecs:      codecs,
        MaxChannels: maxChannels,
        Priority:    priority,
        Weight:      weight,
        Active:      true,
    }
    
    if err := providerMgr.AddProvider(provider); err != nil {
        color.Red("Error: Failed to add provider: %v", err)
        os.Exit(1)
    }
    
    color.Green("✓ Provider '%s' added successfully", name)
    
    // Show provider details
    fmt.Println("\nProvider Details:")
    fmt.Printf("  Type: %s\n", providerType)
    fmt.Printf("  Host: %s:%d\n", host, port)
    
    if username != "" {
        fmt.Printf("  Auth: Username/Password\n")
    } else {
        fmt.Printf("  Auth: IP-based\n")
    }
    
    fmt.Printf("  Codecs: %s\n", strings.Join(codecs, ", "))
    
    if maxChannels > 0 {
        fmt.Printf("  Max Channels: %d\n", maxChannels)
    } else {
        fmt.Printf("  Max Channels: Unlimited\n")
    }
    
    fmt.Printf("  Priority: %d\n", priority)
    fmt.Printf("  Weight: %d\n", weight)
}

func listProviders(cmd *cobra.Command, args []string) {
    filterType, _ := cmd.Flags().GetString("type")
    
    providers, err := providerMgr.ListProviders(filterType)
    if err != nil {
        color.Red("Error: Failed to list providers: %v", err)
        os.Exit(1)
    }
    
    if len(providers) == 0 {
        fmt.Println("No providers found")
        return
    }
    
    table := tablewriter.NewWriter(os.Stdout)
    table.SetHeader([]string{"Name", "Type", "Host:Port", "Auth", "Channels", "Priority", "Weight", "Status"})
    table.SetBorder(true)
    table.SetRowLine(false)
    table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
    table.SetAlignment(tablewriter.ALIGN_LEFT)
    
    for _, p := range providers {
        authType := "IP"
        if p.Username != "" {
            authType = "User/Pass"
        }
        
        channels := "∞"
        if p.MaxChannels > 0 {
            channels = strconv.Itoa(p.MaxChannels)
        }
        
        status := color.GreenString("Active")
        if !p.Active {
            status = color.RedString("Inactive")
        }
        
        table.Append([]string{
            p.Name,
            p.Type,
            fmt.Sprintf("%s:%d", p.Host, p.Port),
            authType,
            channels,
            strconv.Itoa(p.Priority),
            strconv.Itoa(p.Weight),
            status,
        })
    }
    
    table.Render()
    fmt.Printf("\nTotal: %d providers\n", len(providers))
}

func deleteProvider(cmd *cobra.Command, args []string) {
    name := args[0]
    
    // Confirm deletion
    fmt.Printf("Are you sure you want to delete provider '%s'? [y/N]: ", name)
    reader := bufio.NewReader(os.Stdin)
    response, _ := reader.ReadString('\n')
    response = strings.ToLower(strings.TrimSpace(response))
    
    if response != "y" && response != "yes" {
        fmt.Println("Deletion cancelled")
        return
    }
    
    if err := providerMgr.DeleteProvider(name); err != nil {
        color.Red("Error: Failed to delete provider: %v", err)
        os.Exit(1)
    }
    
    color.Green("✓ Provider '%s' deleted successfully", name)
}

func showProvider(cmd *cobra.Command, args []string) {
    name := args[0]
    
    provider, err := providerMgr.GetProvider(name)
    if err != nil {
        color.Red("Error: Provider not found: %v", err)
        os.Exit(1)
    }
    
    fmt.Printf("\nProvider: %s\n", provider.Name)
    fmt.Println(strings.Repeat("-", 40))
    fmt.Printf("Type: %s\n", provider.Type)
    fmt.Printf("Host: %s\n", provider.Host)
    fmt.Printf("Port: %d\n", provider.Port)
    
    if provider.Username != "" {
        fmt.Printf("Authentication: Username/Password\n")
        fmt.Printf("Username: %s\n", provider.Username)
        fmt.Printf("Password: %s\n", strings.Repeat("*", len(provider.Password)))
    } else {
        fmt.Printf("Authentication: IP-based\n")
    }
    
    fmt.Printf("Codecs: %s\n", strings.Join(provider.Codecs, ", "))
    
    if provider.MaxChannels > 0 {
        fmt.Printf("Max Channels: %d\n", provider.MaxChannels)
    } else {
        fmt.Printf("Max Channels: Unlimited\n")
    }
    
    fmt.Printf("Priority: %d\n", provider.Priority)
    fmt.Printf("Weight: %d\n", provider.Weight)
    
    if provider.Active {
        fmt.Printf("Status: %s\n", color.GreenString("Active"))
    } else {
        fmt.Printf("Status: %s\n", color.RedString("Inactive"))
    }
    
    fmt.Printf("Created: %s\n", provider.CreatedAt.Format("2006-01-02 15:04:05"))
    fmt.Printf("Updated: %s\n", provider.UpdatedAt.Format("2006-01-02 15:04:05"))
}

// DID command handlers
func addDIDs(cmd *cobra.Command, args []string) {
    providerName, _ := cmd.Flags().GetString("provider")
    country, _ := cmd.Flags().GetString("country")
    city, _ := cmd.Flags().GetString("city")
    file, _ := cmd.Flags().GetString("file")
    
    if providerName == "" {
        color.Red("Error: Provider name is required (use --provider)")
        os.Exit(1)
    }
    
    // Verify provider exists
    if _, err := providerMgr.GetProvider(providerName); err != nil {
        color.Red("Error: Provider '%s' not found", providerName)
        os.Exit(1)
    }
    
    var didsToAdd []models.DID
    
    if file != "" {
        // Load from CSV file
        f, err := os.Open(file)
        if err != nil {
            color.Red("Error: Failed to open file: %v", err)
            os.Exit(1)
        }
        defer f.Close()
        
        scanner := bufio.NewScanner(f)
        lineNum := 0
        for scanner.Scan() {
            lineNum++
            line := strings.TrimSpace(scanner.Text())
            if line == "" || strings.HasPrefix(line, "#") {
                continue
            }
            
            // Parse CSV line: number,country,city
            parts := strings.Split(line, ",")
            if len(parts) < 1 {
                fmt.Printf("Warning: Skipping invalid line %d: %s\n", lineNum, line)
                continue
            }
            
            did := models.DID{
                Number:       strings.TrimSpace(parts[0]),
                ProviderName: providerName,
                Country:      country,
                City:         city,
            }
            
            // Override country/city if specified in CSV
            if len(parts) > 1 && parts[1] != "" {
                did.Country = strings.TrimSpace(parts[1])
            }
            if len(parts) > 2 && parts[2] != "" {
                did.City = strings.TrimSpace(parts[2])
            }
            
            didsToAdd = append(didsToAdd, did)
        }
        
        if err := scanner.Err(); err != nil {
            color.Red("Error reading file: %v", err)
            os.Exit(1)
        }
        
        fmt.Printf("Loaded %d DIDs from file\n", len(didsToAdd))
    } else if len(args) > 0 {
        // Add DIDs from command line
        for _, number := range args {
            did := models.DID{
                Number:       number,
                ProviderName: providerName,
                Country:      country,
                City:         city,
            }
            didsToAdd = append(didsToAdd, did)
        }
    } else {
        color.Red("Error: No DIDs specified. Use DID numbers as arguments or --file")
        os.Exit(1)
    }
    
    // Add DIDs to database
    success := 0
    failed := 0
    for _, did := range didsToAdd {
        query := `
            INSERT INTO dids (number, provider_name, country, city, in_use, created_at, updated_at)
            VALUES (?, ?, ?, ?, 0, NOW(), NOW())
            ON DUPLICATE KEY UPDATE
                provider_name = VALUES(provider_name),
                country = VALUES(country),
                city = VALUES(city),
                updated_at = NOW()`
        
        if _, err := db.DB.Exec(query, did.Number, did.ProviderName, did.Country, did.City); err != nil {
            color.Red("Failed to add DID %s: %v", did.Number, err)
            failed++
        } else {
            success++
        }
    }
    
    color.Green("✓ Added %d DIDs successfully", success)
    if failed > 0 {
        color.Red("✗ Failed to add %d DIDs", failed)
    }
}

func listDIDs(cmd *cobra.Command, args []string) {
    showAll, _ := cmd.Flags().GetBool("all")
    providerFilter, _ := cmd.Flags().GetString("provider")
    inUse, _ := cmd.Flags().GetBool("in-use")
    available, _ := cmd.Flags().GetBool("available")
    
    query := "SELECT number, provider_name, in_use, destination, country, city FROM dids WHERE 1=1"
    queryArgs := []interface{}{}
    
    if providerFilter != "" {
        query += " AND provider_name = ?"
        queryArgs = append(queryArgs, providerFilter)
    }
    
    if inUse {
        query += " AND in_use = 1"
    } else if available {
        query += " AND in_use = 0"
    }
    
    query += " ORDER BY provider_name, number"
    
    if !showAll && !inUse && !available && providerFilter == "" {
        query += " LIMIT 50"
    }
    
    rows, err := db.DB.Query(query, queryArgs...)
    if err != nil {
        color.Red("Error: Failed to query DIDs: %v", err)
        os.Exit(1)
    }
    defer rows.Close()
    
    table := tablewriter.NewWriter(os.Stdout)
    table.SetHeader([]string{"DID Number", "Provider", "Status", "Destination", "Country", "City"})
    table.SetBorder(true)
    table.SetRowLine(false)
    table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
    table.SetAlignment(tablewriter.ALIGN_LEFT)
    
    count := 0
    for rows.Next() {
        var did models.DID
        var destination sql.NullString
        
        err := rows.Scan(&did.Number, &did.ProviderName, &did.InUse, &destination, &did.Country, &did.City)
        if err != nil {
            continue
        }
        
        status := color.GreenString("Available")
        if did.InUse {
            status = color.RedString("In Use")
        }
        
        destStr := "-"
        if destination.Valid {
            destStr = destination.String
        }
        
        table.Append([]string{
            did.Number,
            did.ProviderName,
            status,
            destStr,
            did.Country,
            did.City,
        })
        count++
    }
    
    table.Render()
    
    if !showAll && count == 50 {
        fmt.Printf("\nShowing first 50 DIDs. Use --all to see all DIDs\n")
    } else {
        fmt.Printf("\nTotal: %d DIDs\n", count)
    }
}

func deleteDID(cmd *cobra.Command, args []string) {
    number := args[0]
    
    // Check if DID is in use
    var inUse bool
    err := db.DB.QueryRow("SELECT in_use FROM dids WHERE number = ?", number).Scan(&inUse)
    if err == sql.ErrNoRows {
        color.Red("Error: DID not found")
        os.Exit(1)
    }
    
    if inUse {
        color.Red("Error: Cannot delete DID %s - currently in use", number)
        os.Exit(1)
    }
    
    if _, err := db.DB.Exec("DELETE FROM dids WHERE number = ?", number); err != nil {
        color.Red("Error: Failed to delete DID: %v", err)
        os.Exit(1)
    }
    
    color.Green("✓ DID '%s' deleted successfully", number)
}

func releaseDID(cmd *cobra.Command, args []string) {
    number := args[0]
    
    query := `UPDATE dids SET in_use = 0, destination = NULL, updated_at = NOW() WHERE number = ?`
    result, err := db.DB.Exec(query, number)
    if err != nil {
        color.Red("Error: Failed to release DID: %v", err)
        os.Exit(1)
    }
    
    rows, _ := result.RowsAffected()
    if rows == 0 {
        color.Red("Error: DID not found")
        os.Exit(1)
    }
    
    color.Green("✓ DID '%s' released successfully", number)
}

// Route command handlers
func addRoute(cmd *cobra.Command, args []string) {
    name := args[0]
    inbound := args[1]
    intermediate := args[2]
    final := args[3]
    
    mode, _ := cmd.Flags().GetString("mode")
    priority, _ := cmd.Flags().GetInt("priority")
    
    // Validate load balance mode
    validModes := []string{"round_robin", "weighted", "priority", "failover"}
    isValidMode := false
    for _, m := range validModes {
        if mode == m {
            isValidMode = true
            break
        }
    }
    
    if !isValidMode {
        color.Red("Error: Invalid load balance mode. Must be: round_robin, weighted, priority, or failover")
        os.Exit(1)
    }
    
    route := &models.ProviderRoute{
        Name:                 name,
        InboundProvider:      inbound,
        IntermediateProvider: intermediate,
        FinalProvider:        final,
        LoadBalanceMode:      mode,
        Priority:             priority,
        Active:               true,
    }
    
    if err := providerMgr.AddProviderRoute(route); err != nil {
        color.Red("Error: Failed to add route: %v", err)
        os.Exit(1)
    }
    
    color.Green("✓ Route '%s' added successfully", name)
    
    // Show route details
    fmt.Println("\nRoute Details:")
    fmt.Printf("  Path: %s → %s → %s\n", inbound, intermediate, final)
    fmt.Printf("  Load Balance Mode: %s\n", mode)
    fmt.Printf("  Priority: %d\n", priority)
}

func listRoutes(cmd *cobra.Command, args []string) {
    query := `
        SELECT name, inbound_provider, intermediate_provider, final_provider, 
               load_balance_mode, priority, active
        FROM provider_routes
        ORDER BY priority DESC, name`
    
    rows, err := db.DB.Query(query)
    if err != nil {
        color.Red("Error: Failed to query routes: %v", err)
        os.Exit(1)
    }
    defer rows.Close()
    
    table := tablewriter.NewWriter(os.Stdout)
    table.SetHeader([]string{"Name", "Inbound", "Intermediate", "Final", "Mode", "Priority", "Status"})
    table.SetBorder(true)
    table.SetRowLine(false)
    table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
    table.SetAlignment(tablewriter.ALIGN_LEFT)
    
    count := 0
    for rows.Next() {
        var route models.ProviderRoute
        err := rows.Scan(&route.Name, &route.InboundProvider, &route.IntermediateProvider,
            &route.FinalProvider, &route.LoadBalanceMode, &route.Priority, &route.Active)
        if err != nil {
            continue
        }
        
        status := color.GreenString("Active")
        if !route.Active {
            status = color.RedString("Inactive")
        }
        
        table.Append([]string{
            route.Name,
            route.InboundProvider,
            route.IntermediateProvider,
            route.FinalProvider,
            route.LoadBalanceMode,
            strconv.Itoa(route.Priority),
            status,
        })
        count++
    }
    
    table.Render()
    fmt.Printf("\nTotal: %d routes\n", count)
}

func deleteRoute(cmd *cobra.Command, args []string) {
    name := args[0]
    
    // Confirm deletion
    fmt.Printf("Are you sure you want to delete route '%s'? [y/N]: ", name)
    reader := bufio.NewReader(os.Stdin)
    response, _ := reader.ReadString('\n')
    response = strings.ToLower(strings.TrimSpace(response))
    
    if response != "y" && response != "yes" {
        fmt.Println("Deletion cancelled")
        return
    }
    
    if _, err := db.DB.Exec("DELETE FROM provider_routes WHERE name = ?", name); err != nil {
        color.Red("Error: Failed to delete route: %v", err)
        os.Exit(1)
    }
    
    color.Green("✓ Route '%s' deleted successfully", name)
}

func showRoute(cmd *cobra.Command, args []string) {
    name := args[0]
    
    var route models.ProviderRoute
    query := `
        SELECT name, inbound_provider, intermediate_provider, final_provider, 
               load_balance_mode, priority, active, created_at
        FROM provider_routes
        WHERE name = ?`
    
    err := db.DB.QueryRow(query, name).Scan(
        &route.Name, &route.InboundProvider, &route.IntermediateProvider,
        &route.FinalProvider, &route.LoadBalanceMode, &route.Priority,
        &route.Active, &route.CreatedAt)
    
    if err == sql.ErrNoRows {
        color.Red("Error: Route not found")
        os.Exit(1)
    } else if err != nil {
        color.Red("Error: Failed to query route: %v", err)
        os.Exit(1)
    }
    
    fmt.Printf("\nRoute: %s\n", route.Name)
    fmt.Println(strings.Repeat("-", 40))
    fmt.Printf("Path: %s → %s → %s\n", 
        route.InboundProvider, route.IntermediateProvider, route.FinalProvider)
    fmt.Printf("Load Balance Mode: %s\n", route.LoadBalanceMode)
    fmt.Printf("Priority: %d\n", route.Priority)
    
    if route.Active {
        fmt.Printf("Status: %s\n", color.GreenString("Active"))
    } else {
        fmt.Printf("Status: %s\n", color.RedString("Inactive"))
    }
    
    fmt.Printf("Created: %s\n", route.CreatedAt.Format("2006-01-02 15:04:05"))
}

// Stats command handlers
func showStats(cmd *cobra.Command, args []string) {
    showProviders, _ := cmd.Flags().GetBool("providers")
    showCalls, _ := cmd.Flags().GetBool("calls")
    showDIDs, _ := cmd.Flags().GetBool("dids")
    
    // If no specific flag, show all
    if !showProviders && !showCalls && !showDIDs {
        showProviders = true
        showCalls = true
        showDIDs = true
    }
    
    // Get router statistics
    stats := providerMgr.GetRouterStats()
    
    if showDIDs {
        fmt.Println("\n=== DID Statistics ===")
        fmt.Printf("Total DIDs: %d\n", stats["total_dids"])
        fmt.Printf("Used DIDs: %d\n", stats["used_dids"])
        fmt.Printf("Available DIDs: %d\n", stats["available_dids"])
        
        if total, ok := stats["total_dids"].(int); ok && total > 0 {
            used := stats["used_dids"].(int)
            utilization := float64(used) / float64(total) * 100
            fmt.Printf("Utilization: %.1f%%\n", utilization)
        }
    }
    
    if showCalls {
        fmt.Println("\n=== Call Statistics ===")
        fmt.Printf("Active Calls: %d\n", stats["active_calls"])
        
        // Get recent call statistics
        var totalCalls, completedCalls, failedCalls int
        db.DB.QueryRow(`
            SELECT 
                COUNT(*) as total,
                SUM(CASE WHEN status = 'COMPLETED' THEN 1 ELSE 0 END) as completed,
                SUM(CASE WHEN status = 'FAILED' THEN 1 ELSE 0 END) as failed
            FROM call_records
            WHERE start_time >= DATE_SUB(NOW(), INTERVAL 24 HOUR)
        `).Scan(&totalCalls, &completedCalls, &failedCalls)
        
        fmt.Printf("Last 24h: %d total, %d completed, %d failed\n", 
            totalCalls, completedCalls, failedCalls)
        
        if totalCalls > 0 {
            successRate := float64(completedCalls) / float64(totalCalls) * 100
            fmt.Printf("Success Rate: %.1f%%\n", successRate)
        }
    }
    
    if showProviders {
        fmt.Println("\n=== Provider Statistics ===")
        
        query := `
            SELECT provider_name, total_calls, active_calls, failed_calls, 
                   success_rate, avg_call_duration, is_healthy
            FROM provider_stats
            ORDER BY provider_name`
        
        rows, err := db.DB.Query(query)
        if err == nil {
            defer rows.Close()
            
            table := tablewriter.NewWriter(os.Stdout)
            table.SetHeader([]string{"Provider", "Total", "Active", "Failed", "Success%", "Avg Duration", "Health"})
            table.SetBorder(true)
            
            for rows.Next() {
                var name string
                var total, active, failed int64
                var successRate, avgDuration float64
                var isHealthy bool
                
                err := rows.Scan(&name, &total, &active, &failed, &successRate, &avgDuration, &isHealthy)
                if err != nil {
                    continue
                }
                
                health := color.GreenString("✓")
                if !isHealthy {
                    health = color.RedString("✗")
                }
                
                table.Append([]string{
                    name,
                    strconv.FormatInt(total, 10),
                    strconv.FormatInt(active, 10),
                    strconv.FormatInt(failed, 10),
                    fmt.Sprintf("%.1f%%", successRate),
                    fmt.Sprintf("%.1fs", avgDuration),
                    health,
                })
            }
            
            table.Render()
        }
    }
}

func showLoadBalancer(cmd *cobra.Command, args []string) {
    fmt.Println("\n=== Load Balancer Status ===")
    
    // Show active routes and their current selection
    query := `
        SELECT r.name, r.inbound_provider, r.intermediate_provider, r.final_provider, 
               r.load_balance_mode, r.priority
        FROM provider_routes r
        WHERE r.active = 1
        ORDER BY r.priority DESC`
    
    rows, err := db.DB.Query(query)
    if err != nil {
        color.Red("Error: Failed to query routes: %v", err)
        return
    }
    defer rows.Close()
    
    for rows.Next() {
        var route models.ProviderRoute
        err := rows.Scan(&route.Name, &route.InboundProvider, &route.IntermediateProvider,
            &route.FinalProvider, &route.LoadBalanceMode, &route.Priority)
        if err != nil {
            continue
        }
        
        fmt.Printf("\nRoute: %s (Mode: %s, Priority: %d)\n", 
            route.Name, route.LoadBalanceMode, route.Priority)
        fmt.Printf("  Path: %s → %s → %s\n", 
            route.InboundProvider, route.IntermediateProvider, route.FinalProvider)
        
        // Show provider health for this route
        fmt.Printf("  Provider Health:\n")
        for _, provider := range []string{route.InboundProvider, route.IntermediateProvider, route.FinalProvider} {
            var isHealthy bool
            var activeCalls int
            err := db.DB.QueryRow(`
                SELECT is_healthy, active_calls 
                FROM provider_stats 
                WHERE provider_name = ?
            `, provider).Scan(&isHealthy, &activeCalls)
            
            if err == nil {
                health := color.GreenString("Healthy")
                if !isHealthy {
                    health = color.RedString("Unhealthy")
                }
                fmt.Printf("    %s: %s (Active: %d)\n", provider, health, activeCalls)
            } else {
                fmt.Printf("    %s: No data\n", provider)
            }
        }
    }
}

func showCalls(cmd *cobra.Command, args []string) {
    limit, _ := cmd.Flags().GetInt("limit")
    status, _ := cmd.Flags().GetString("status")
    
    query := `
        SELECT call_id, original_ani, original_dnis, transformed_ani, assigned_did,
               inbound_provider, intermediate_provider, final_provider, status, 
               current_step, start_time, duration
        FROM call_records
        WHERE 1=1`
    
    queryArgs := []interface{}{}
    
    if status != "" {
        query += " AND status = ?"
        queryArgs = append(queryArgs, status)
    }
    
    query += " ORDER BY start_time DESC LIMIT ?"
    queryArgs = append(queryArgs, limit)
    
    rows, err := db.DB.Query(query, queryArgs...)
    if err != nil {
        color.Red("Error: Failed to query calls: %v", err)
        return
    }
    defer rows.Close()
    
    table := tablewriter.NewWriter(os.Stdout)
    table.SetHeader([]string{"Call ID", "ANI", "DNIS", "DID", "Status", "Step", "Duration", "Time"})
    table.SetBorder(true)
    table.SetRowLine(false)
    
    for rows.Next() {
        var record models.CallRecord
        var duration sql.NullInt64
        
        err := rows.Scan(&record.CallID, &record.OriginalANI, &record.OriginalDNIS,
            &record.TransformedANI, &record.AssignedDID, &record.InboundProvider,
            &record.IntermediateProvider, &record.FinalProvider, &record.Status,
            &record.CurrentStep, &record.StartTime, &duration)
        if err != nil {
            continue
        }
        
        durationStr := "-"
        if duration.Valid {
            durationStr = fmt.Sprintf("%ds", duration.Int64)
        }
        
        statusColor := record.Status
        switch record.Status {
        case "COMPLETED":
            statusColor = color.GreenString(record.Status)
        case "ACTIVE":
            statusColor = color.YellowString(record.Status)
        case "FAILED", "ABANDONED":
            statusColor = color.RedString(record.Status)
        }
        
        table.Append([]string{
            record.CallID[:8] + "...",
            record.OriginalANI,
            record.OriginalDNIS,
            record.AssignedDID,
            statusColor,
            record.CurrentStep,
            durationStr,
            record.StartTime.Format("15:04:05"),
        })
    }
    
    table.Render()
}

func monitorSystem(cmd *cobra.Command, args []string) {
    fmt.Println("Starting system monitor... Press Ctrl+C to exit")
    fmt.Println()
    
    for {
        // Clear screen (works on Unix-like systems)
        fmt.Print("\033[H\033[2J")
        
        // Show current time
        fmt.Printf("System Monitor - %s\n", time.Now().Format("2006-01-02 15:04:05"))
        fmt.Println(strings.Repeat("=", 60))
        
        // Get statistics
        stats := providerMgr.GetRouterStats()
        
        // Show active calls
        fmt.Printf("\nActive Calls: %d\n", stats["active_calls"])
        
        // Show DID usage
        fmt.Printf("\nDID Usage: %d/%d (%.1f%% utilized)\n",
            stats["used_dids"], stats["total_dids"],
            float64(stats["used_dids"].(int))/float64(stats["total_dids"].(int))*100)
        
        // Show provider status
        fmt.Println("\nProvider Status:")
        query := `
            SELECT provider_name, active_calls, is_healthy
            FROM provider_stats
            WHERE active_calls > 0 OR last_call_time > DATE_SUB(NOW(), INTERVAL 5 MINUTE)
            ORDER BY active_calls DESC`
        
        rows, err := db.DB.Query(query)
        if err == nil {
            defer rows.Close()
            
            for rows.Next() {
                var name string
                var activeCalls int
                var isHealthy bool
                
                rows.Scan(&name, &activeCalls, &isHealthy)
                
                health := color.GreenString("[OK]")
                if !isHealthy {
                    health = color.RedString("[FAIL]")
                }
                
                fmt.Printf("  %-20s %s Active: %d\n", name, health, activeCalls)
            }
        }
        
        // Show recent calls
        fmt.Println("\nRecent Calls:")
        callQuery := `
            SELECT call_id, status, current_step, 
                   TIMESTAMPDIFF(SECOND, start_time, NOW()) as elapsed
            FROM call_records
            WHERE status = 'ACTIVE'
            ORDER BY start_time DESC
            LIMIT 5`
        
        rows, err = db.DB.Query(callQuery)
        if err == nil {
            defer rows.Close()
            
            for rows.Next() {
                var callID, status, step string
                var elapsed int
                
                rows.Scan(&callID, &status, &step, &elapsed)
                fmt.Printf("  %s: %s (%s) - %ds\n", 
                    callID[:8]+"...", status, step, elapsed)
            }
        }
        
        // Sleep for 2 seconds before refreshing
        time.Sleep(2 * time.Second)
    }
}
