package gocodegen

import (
	"bufio"
	"bytes"
	"fmt"
	"gomachine/config"
	"gomachine/util"
	"os"
	"path/filepath"
	"strings"

	"github.com/CloudyKit/jet/v6"
	log "github.com/sirupsen/logrus"
)

type RouteGroup struct {
	Name   string
	Path   string
	Routes []Route
}

type Route struct {
	Method          string
	URL             string
	ParamCode       string
	ControllerName  string
	ControllerBase  string
	FuncName        string
	ParamStr        string
	PreFlag         bool
	PostFlag        bool
	NeedsBodyParser bool
}

type RouterData struct {
	PackageName string
	Auth        string
	JsonFlag    bool
	UrlImport   bool
	Apis        bool
	Cassandra   bool
	Groups      []RouteGroup
	Imports     []string
}

func GenerateGoRouter(packageName string, cnf config.ModelConfig) {
	path := fmt.Sprintf("%v/bin/buildtool", os.Getenv("HOME"))
	var views = jet.NewSet(jet.NewOSFileSystemLoader(path), jet.InDevelopmentMode())

	// Add global functions for templates
	views.AddGlobal("title", func(str string) string {
		return strings.Title(str)
	})

	// Controller analysis data
	routerData := RouterData{
		PackageName: packageName,
		Auth:        cnf.Auth,
	}

	// Analyze controllers and generate routes
	domainRoutes := analyzeControllersByDomain(packageName, cnf, &routerData)

	// Create router directory if it doesn't exist
	routerDir := cnf.GoModelFilePath + "/router"
	if err := os.MkdirAll(routerDir, 0755); err != nil {
		log.Printf("Failed to create router directory %s: %v", routerDir, err)
		return
	}

	if err := os.MkdirAll(routerDir+"/routers", 0755); err != nil {
		log.Printf("Failed to create router directory %s: %v", routerDir+"/routers", err)
		return
	}

	// Generate domain-specific router files
	domains := make([]string, 0)
	for domainName, routes := range domainRoutes {
		domains = append(domains, domainName)

		// Determine controller type and if log is needed
		controllerType := "rest"
		needsLog := false
		for _, route := range routes {
			if strings.Contains(route.ControllerName, "api.") {
				controllerType = "api"
			}
			if route.NeedsBodyParser {
				needsLog = true
			}
		}

		domainData := make(jet.VarMap)
		domainData.Set("packageName", packageName)
		domainData.Set("domainName", domainName)
		domainData.Set("controllerType", controllerType)
		domainData.Set("needsLog", needsLog)
		domainData.Set("routes", routes)

		var domainBuffer bytes.Buffer
		domainTemplate, err := views.GetTemplate("go/domain_router.jet")
		if err != nil {
			log.Printf("CRITICAL ERROR: Failed to load domain router template: %v", err)
			continue
		}

		if err = domainTemplate.Execute(&domainBuffer, domainData, nil); err != nil {
			log.Printf("CRITICAL ERROR: Domain router template execution failed for %s: %v", domainName, err)
			continue
		}

		domainFile := routerDir + "/routers/" + strings.ToLower(domainName) + ".go"
		if err := util.WriteFile(domainFile, domainBuffer.String()); err != nil {
			log.Printf("CRITICAL ERROR: Failed to write domain router file %s: %v", domainFile, err)
		} else {
			log.Printf("SUCCESS: Domain router file written: %s", domainFile)
		}
	}

	// Generate main router file
	mainData := make(jet.VarMap)
	mainData.Set("packageName", packageName)
	mainData.Set("auth", routerData.Auth)
	mainData.Set("jsonFlag", routerData.JsonFlag)
	mainData.Set("urlImport", routerData.UrlImport)
	mainData.Set("apis", routerData.Apis)
	mainData.Set("cassandra", routerData.Cassandra)
	mainData.Set("imports", routerData.Imports)
	mainData.Set("domains", domains)

	var mainBuffer bytes.Buffer
	mainTemplate, err := views.GetTemplate("go/router.jet")
	if err != nil {
		log.Printf("CRITICAL ERROR: Failed to load main router template: %v", err)
		return
	}

	if err = mainTemplate.Execute(&mainBuffer, mainData, nil); err != nil {
		log.Printf("CRITICAL ERROR: Main router template execution failed: %v", err)
		return
	}

	mainRouterFile := routerDir + "/router.go"
	if err := util.WriteFile(mainRouterFile, mainBuffer.String()); err != nil {
		log.Printf("CRITICAL ERROR: Failed to write main router file %s: %v", mainRouterFile, err)
	} else {
		log.Printf("SUCCESS: Main router file written: %s", mainRouterFile)
	}

	log.Printf("Generated router files for %d domains: %v", len(domains), domains)
}

func analyzeControllersByDomain(packageName string, cnf config.ModelConfig, routerData *RouterData) map[string][]Route {
	controllerPath := cnf.GoModelFilePath + "/controllers"

	// Check if controllers directory exists
	if _, err := os.Stat(controllerPath); os.IsNotExist(err) {
		log.Printf("Controllers directory not found: %s", controllerPath)
		return make(map[string][]Route)
	}

	// Get list of all controller files and their functions
	apiPath := controllerPath + "/api"
	restPath := controllerPath + "/rest"

	controllerFunctions := make(map[string]map[string][]string) // [type][controller] -> functions
	controllerFunctions["api"] = make(map[string][]string)
	controllerFunctions["rest"] = make(map[string][]string)

	// Parse API controllers
	if _, err := os.Stat(apiPath); err == nil {
		parseControllersInDirectory(apiPath, "api", controllerFunctions["api"])
	}

	// Parse REST controllers
	if _, err := os.Stat(restPath); err == nil {
		parseControllersInDirectory(restPath, "rest", controllerFunctions["rest"])
	}

	// Generate routes with priority: API first, then REST
	allControllers := make(map[string]bool)

	// Collect all controller names
	for controller := range controllerFunctions["api"] {
		allControllers[controller] = true
	}
	for controller := range controllerFunctions["rest"] {
		allControllers[controller] = true
	}

	// Group routes by domain (controller name)
	domainRoutes := make(map[string][]Route)

	// Generate routes for each controller
	for controllerName := range allControllers {
		apiFunctions := controllerFunctions["api"][controllerName]
		restFunctions := controllerFunctions["rest"][controllerName]

		// Determine which controller type to use for each function
		routes := generateSmartRoutes(controllerName, apiFunctions, restFunctions)
		domainRoutes[controllerName] = routes
	}

	if len(controllerFunctions["api"]) > 0 {
		routerData.Apis = true
	}
	routerData.JsonFlag = true

	totalRoutes := 0
	for _, routes := range domainRoutes {
		totalRoutes += len(routes)
	}
	log.Printf("Analyzed controllers, found %d routes across %d domains", totalRoutes, len(domainRoutes))

	return domainRoutes
}

func parseControllersInDirectory(dirPath, controllerType string, result map[string][]string) {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		return
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".go") {
			continue
		}

		controllerName := strings.TrimSuffix(file.Name(), ".go")
		filePath := filepath.Join(dirPath, file.Name())

		functions, err := parseControllerFunctions(filePath)
		if err != nil {
			log.Printf("Error parsing controller %s: %v", filePath, err)
			continue
		}

		result[controllerName] = functions
	}
}

func generateSmartRoutes(controllerName string, apiFunctions, restFunctions []string) []Route {
	routes := []Route{}

	// Collect all unique functions
	allFunctions := make(map[string]bool)
	for _, fn := range apiFunctions {
		allFunctions[fn] = true
	}
	for _, fn := range restFunctions {
		allFunctions[fn] = true
	}

	// Generate routes for each function
	for funcName := range allFunctions {
		// Check if function exists in API controller first
		var controllerType string
		if contains(apiFunctions, funcName) {
			controllerType = "api"
		} else if contains(restFunctions, funcName) {
			controllerType = "rest"
		} else {
			continue // Should not happen
		}

		route := generateRouteForFunction(controllerName, controllerType, funcName)
		routes = append(routes, route)
	}

	return routes
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func generateRouteForFunction(controllerName, controllerType, funcName string) Route {
	controllerClass := controllerType + "." + strings.Title(controllerName) + "Controller"
	controllerBase := strings.Title(controllerName)
	funcLower := strings.ToLower(funcName)

	// Determine HTTP method and URL based on function name
	if funcName == "Insert" || funcName == "Create" {
		return Route{
			Method:          "Post",
			URL:             "/" + strings.ToLower(controllerName),
			ParamCode:       generateParamCode(controllerName, "Post", controllerType, ""),
			ControllerName:  controllerClass,
			ControllerBase:  controllerBase,
			FuncName:        funcName,
			ParamStr:        "item_",
			NeedsBodyParser: true,
		}
	} else if funcName == "Insertbatch" {
		paramStr := "item_"
		if strings.ToLower(controllerName) == "user" {
			paramStr = "items_" // User uses items_ (same as others)
		}
		return Route{
			Method:          "Post",
			URL:             "/" + strings.ToLower(controllerName) + "/batch",
			ParamCode:       generateParamCode(controllerName, "Insertbatch", controllerType, ""),
			ControllerName:  controllerClass,
			ControllerBase:  controllerBase,
			FuncName:        funcName,
			ParamStr:        paramStr,
			NeedsBodyParser: true,
		}
	} else if funcName == "Update" {
		return Route{
			Method:          "Put",
			URL:             "/" + strings.ToLower(controllerName),
			ParamCode:       generateParamCode(controllerName, "Put", controllerType, ""),
			ControllerName:  controllerClass,
			ControllerBase:  controllerBase,
			FuncName:        funcName,
			ParamStr:        "item_",
			NeedsBodyParser: true,
		}
	} else if funcName == "Delete" {
		return Route{
			Method:          "Delete",
			URL:             "/" + strings.ToLower(controllerName),
			ParamCode:       generateParamCode(controllerName, "Delete", controllerType, ""),
			ControllerName:  controllerClass,
			ControllerBase:  controllerBase,
			FuncName:        funcName,
			ParamStr:        "item_",
			NeedsBodyParser: true,
		}
	} else if funcName == "Deletebatch" {
		return Route{
			Method:          "Delete",
			URL:             "/" + strings.ToLower(controllerName) + "/batch",
			ParamCode:       generateParamCode(controllerName, "Deletebatch", controllerType, ""),
			ControllerName:  controllerClass,
			ControllerBase:  controllerBase,
			FuncName:        funcName,
			ParamStr:        "item_",
			NeedsBodyParser: true,
		}
	} else if funcName == "Get" || funcName == "Read" {
		return Route{
			Method:         "Get",
			URL:            "/" + strings.ToLower(controllerName) + "/:id",
			ParamCode:      generateParamCode(controllerName, "Get", controllerType, "id"),
			ControllerName: controllerClass,
			ControllerBase: controllerBase,
			FuncName:       funcName,
			ParamStr:       "id_",
		}
	} else if funcName == "Index" || funcName == "List" {
		return Route{
			Method:         "Get",
			URL:            "/" + strings.ToLower(controllerName),
			ParamCode:      generateParamCode(controllerName, "Index", controllerType, ""),
			ControllerName: controllerClass,
			ControllerBase: controllerBase,
			FuncName:       funcName,
			ParamStr:       "page_, pagesize_",
		}
	} else if strings.HasPrefix(funcLower, "getby") {
		param := strings.ToLower(funcName[5:])
		return Route{
			Method:         "Get",
			URL:            "/" + strings.ToLower(controllerName) + "/get/" + param + "/:" + param,
			ParamCode:      generateParamCode(controllerName, "Get", controllerType, param),
			ControllerName: controllerClass,
			ControllerBase: controllerBase,
			FuncName:       funcName,
			ParamStr:       param + "_",
		}
	} else if strings.HasPrefix(funcLower, "findby") {
		param := strings.ToLower(funcName[6:])
		return Route{
			Method:         "Get",
			URL:            "/" + strings.ToLower(controllerName) + "/find/" + param + "/:" + param,
			ParamCode:      generateParamCode(controllerName, "Get", controllerType, param),
			ControllerName: controllerClass,
			ControllerBase: controllerBase,
			FuncName:       funcName,
			ParamStr:       param + "_",
		}
	} else if strings.HasPrefix(funcLower, "countby") {
		param := strings.ToLower(funcName[7:])
		return Route{
			Method:         "Get",
			URL:            "/" + strings.ToLower(controllerName) + "/count/" + param + "/:" + param,
			ParamCode:      generateParamCode(controllerName, "Get", controllerType, param),
			ControllerName: controllerClass,
			ControllerBase: controllerBase,
			FuncName:       funcName,
			ParamStr:       param + "_",
		}
	} else if strings.HasPrefix(funcLower, "update") && strings.HasSuffix(funcLower, "byid") {
		// Handle Update{Field}ById patterns like UpdateLogindateById, UpdatePointById
		fieldName := strings.ToLower(funcName[6 : len(funcName)-4]) // Remove "Update" and "ById"
		return Route{
			Method:          "Put",
			URL:             "/" + strings.ToLower(controllerName) + "/" + fieldName + "byid",
			ParamCode:       generateParamCode(controllerName, "UpdateByField", controllerType, fieldName),
			ControllerName:  controllerClass,
			ControllerBase:  controllerBase,
			FuncName:        funcName,
			ParamStr:        fieldName + "_, id_",
			NeedsBodyParser: true,
		}
	} else {
		// Generic function
		return Route{
			Method:         "Post",
			URL:            "/" + strings.ToLower(controllerName) + "/" + strings.ToLower(funcName),
			ParamCode:      "",
			ControllerName: controllerClass,
			ControllerBase: controllerBase,
			FuncName:       funcName,
			ParamStr:       "",
		}
	}
}

func parseControllerFunctions(filePath string) ([]string, error) {
	functions := []string{}

	file, err := os.Open(filePath)
	if err != nil {
		return functions, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Look for function definitions like: func (c *Controller) FunctionName(
		if strings.HasPrefix(line, "func (") && strings.Contains(line, ")") {
			// Extract function name
			parts := strings.Split(line, ")")
			if len(parts) >= 2 {
				funcPart := strings.TrimSpace(parts[1])
				if idx := strings.Index(funcPart, "("); idx > 0 {
					funcName := strings.TrimSpace(funcPart[:idx])
					if funcName != "" && !strings.HasPrefix(funcName, "Init") && !strings.HasPrefix(funcName, "Close") {
						functions = append(functions, funcName)
					}
				}
			}
		}
	}

	return functions, scanner.Err()
}

func generateParamCode(controllerName, method, controllerType, param string) string {
	// Special handling for User controller
	if strings.ToLower(controllerName) == "user" {
		if controllerType == "api" {
			// api.UserController uses UserUpdate model
			if method == "Post" || method == "Put" || method == "Delete" {
				return "\t\t\titem_ := &models.UserUpdate{}\n\t\t\terr := c.BodyParser(item_)\n\t\t\tif err != nil {\n\t\t\t    log.Error().Msg(err.Error())\n\t\t\t}"
			}
			if method == "Insertbatch" {
				return "\t\t\tvar results map[string]interface{}\n\t\t\tjsonData := c.Body()\n\t\t\tjsonErr := json.Unmarshal(jsonData, &results)\n\t\t\tif jsonErr != nil {\n\t\t\t    log.Error().Msg(jsonErr.Error())\n\t\t\t}\n\t\t\tvar items_ *[]models.UserUpdate\n\t\t\titems__ref := &items_\n\t\t\terr := c.BodyParser(items__ref)\n\t\t\tif err != nil {\n\t\t\t    log.Error().Msg(err.Error())\n\t\t\t}"
			}
		} else if controllerType == "rest" {
			// rest.UserController uses User model
			if method == "Post" || method == "Put" {
				return fmt.Sprintf("\t\t\titem_ := &models.%sUpdate{}\n\t\t\terr := c.BodyParser(item_)\n\t\t\tif err != nil {\n\t\t\t    log.Error().Msg(err.Error())\n\t\t\t}", strings.Title(controllerName))
			}
			if method == "Insertbatch" {
				return "\t\t\tvar results map[string]interface{}\n\t\t\tjsonData := c.Body()\n\t\t\tjsonErr := json.Unmarshal(jsonData, &results)\n\t\t\tif jsonErr != nil {\n\t\t\t    log.Error().Msg(jsonErr.Error())\n\t\t\t}\n\t\t\tvar items_ *[]models.UserUpdate\n\t\t\titems__ref := &items_\n\t\t\terr := c.BodyParser(items__ref)\n\t\t\tif err != nil {\n\t\t\t    log.Error().Msg(err.Error())\n\t\t\t}"
			}
			if method == "Deletebatch" {
				return "\t\t\titem_ := &[]models.User{}\n\t\t\terr := c.BodyParser(item_)\n\t\t\tif err != nil {\n\t\t\t    log.Error().Msg(err.Error())\n\t\t\t}"
			}
		}
	}

	// General controller handling
	if method == "Post" || method == "Put" || method == "Delete" {
		return fmt.Sprintf("\t\t\titem_ := &models.%s{}\n\t\t\terr := c.BodyParser(item_)\n\t\t\tif err != nil {\n\t\t\t    log.Error().Msg(err.Error())\n\t\t\t}", strings.Title(controllerName))
	}
	if method == "Insertbatch" || method == "Deletebatch" {
		return fmt.Sprintf("\t\t\titem_ := &[]models.%s{}\n\t\t\terr := c.BodyParser(item_)\n\t\t\tif err != nil {\n\t\t\t    log.Error().Msg(err.Error())\n\t\t\t}", strings.Title(controllerName))
	}
	if method == "Get" {
		// Check for specific parameter types
		if param == "loginid" || param == "connectid" || param == "email" {
			return fmt.Sprintf("\t\t\t%s_ := c.Params(\"%s\")", param, param)
		}
		if param == "level" {
			return fmt.Sprintf("\t\t\tvar %s_ user.Level\n\t\t\t%s__, _ := strconv.Atoi(c.Params(\"%s\"))\n\t\t\t%s_ = user.Level(%s__)", param, param, param, param, param)
		}
		// Default: assume it's an ID parameter
		if param == "id" {
			return "\t\t\tid_, _ := strconv.ParseInt(c.Params(\"id\"), 10, 64)"
		}
		// Generic parameter handling - assume int64 type for unknown parameters
		return fmt.Sprintf("\t\t\t%s_, _ := strconv.ParseInt(c.Params(\"%s\"), 10, 64)", param, param)
	}
	if method == "Index" {
		return "\t\t\tpage_, _ := strconv.Atoi(c.Query(\"page\"))\n\t\t\tpagesize_, _ := strconv.Atoi(c.Query(\"pagesize\"))"
	}
	if method == "UpdateByField" {
		// Handle Update{Field}ById patterns with JSON body parsing
		var fieldType string
		switch param {
		case "logindate":
			fieldType = "string"
		case "point":
			fieldType = "int"
		default:
			fieldType = "string" // Default to string
		}

		var fieldExtraction string
		if fieldType == "string" {
			fieldExtraction = fmt.Sprintf("\t\t\tvar %s_ string\n\t\t\tif v, flag := results[\"%s\"]; flag {\n\t\t\t\t%s_ = v.(string)\n\t\t\t}", param, param, param)
		} else if fieldType == "int" {
			fieldExtraction = fmt.Sprintf("\t\t\tvar %s_ int\n\t\t\tif v, flag := results[\"%s\"]; flag {\n\t\t\t\t%s_ = int(v.(float64))\n\t\t\t}", param, param, param)
		}

		return fmt.Sprintf("\t\t\tvar results map[string]interface{}\n\t\t\tjsonData := c.Body()\n\t\t\tjsonErr := json.Unmarshal(jsonData, &results)\n\t\t\tif jsonErr != nil {\n\t\t\t    log.Error().Msg(jsonErr.Error())\n\t\t\t}\n%s\n\t\t\tvar id_ int64\n\t\t\tif v, flag := results[\"id\"]; flag {\n\t\t\t\tid_ = int64(v.(float64))\n\t\t\t}", fieldExtraction)
	}
	return ""
}