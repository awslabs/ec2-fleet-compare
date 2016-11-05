package main

import (
	"fmt"
	"github.com/codegangsta/cli"
	"github.com/mitchellh/go-homedir"
	"github.com/olekukonko/tablewriter"
	"github.com/dustin/go-humanize"
	"time"
	"net/http"
	"os"
	"strconv"
	"encoding/json"
	"errors"
	"regexp"
	"io/ioutil"
	"strings"
	"sort"
	// "github.com/davecgh/go-spew/spew"
)

var cacheDir = ".ec2FleetCompare"
var ec2PricesURL string = "https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonEC2/current/index.json";
var ec2SpotPricesURL string = "https://spot-price.s3.amazonaws.com/spot.js"

// var ec2PricesURL string = "http://localhost:1313/ec2_demand_prices.json"
// var ec2SpotPricesURL string = "http://localhost:1313/spot.js"


var ec2RegionMap = map[string]string{
	"AWS GovCloud (US)": 					"gov-west-1",
	"Asia Pacific (Seoul)": 			"ap-northeast-2",
	"Asia Pacific (Singapore)": 	"ap-southeast-1",
	"Asia Pacific (Sydney)": 			"ap-southeast-2",
	"Asia Pacific (Tokyo)":				"ap-northeast-1",
	"EU (Frankfurt)": 						"eu-central-1",
	"EU (Ireland)": 							"eu-west-1",
	"South America (Sao Paulo)": 	"sa-east-1",
	"US East (N. Virginia)": 			"us-east-1",
	"US West (N. California)": 		"us-west-1",
	"US West (Oregon)": 					"us-west-2",
}

var ec2RSpotegionMap = map[string]string{
	"AWS GovCloud (US)": 	"gov-west-1",
	"ap-northeast-2": 		"ap-northeast-2",
	"apac-sin": 					"ap-southeast-1",
	"apac-syd": 					"ap-southeast-2",
	"apac-tokyo":					"ap-northeast-1",
	"eu-central-1": 			"eu-central-1",
	"eu-ireland": 				"eu-west-1",
	"sa-east-1": 					"sa-east-1",
	"us-east": 						"us-east-1",
	"us-west": 						"us-west-1",
	"us-west-2": 					"us-west-2",
}

var networkMap = map[string]int{
	"any": 	4,
	"low": 	4,
	"med": 	3,
	"high": 2,
	"gbit": 1,
}

type InstanceSpecs struct {
	Mem         float64
	Cpu         int
	Os					string
	CpuClock		string
	DiskSize		int
	DiskType		string
	NetworkType int
	NetworkDesc	string
	Description string
}

type Instance struct {
	Sku											string
	Name										string
	RegionName							string
	RegionCode							string
	Specs 									InstanceSpecs
	DemandPrice 						float64
	Reserve1YPartialPrice 	float64
	Reserve1YPartialUpfront float64
	Reserve1YZeroPrice 			float64
	Reserve1YFullUpfront 		float64
	Reserve3YPartialPrice 	float64
	Reserve3YPartialUpfront float64
	Reserve3YFullUpfront 		float64
	SpotPrice 							float64
}

type Ec2 struct {
	Instance []Instance
}

type Ec2Filtered struct {
	NumberInstances		int
	SortPrice					float64
	TotalPriceDemand	float64
	TotalPriceRI			float64
	TotalPriceSpot		float64
	Instance					Instance
}

type FilteredResults []Ec2Filtered

func (slice FilteredResults) Len() int {
  return len(slice)
}

func (slice FilteredResults) Less(i, j int) bool {
  	return slice[i].SortPrice < slice[j].SortPrice
}

func (slice FilteredResults) Swap(i, j int) {
  slice[i], slice[j] = slice[j], slice[i]
}


func printError(s string) {
	fmt.Println("***************************** ERROR ********************************************")
	fmt.Printf("ERROR: %s\n", s)
	fmt.Println("********************************************************************************\n\n")
}

func getJson(url string, target interface{}, jsonp bool) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if jsonp {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		r := regexp.MustCompile(`(?s)callback\s*\((.*)\)`)
		jsonBytes := r.FindSubmatch(body)
		if jsonBytes == nil || len(jsonBytes) < 2 {
			return errors.New("Could not decode JSONP callback")
		}

		return json.Unmarshal(jsonBytes[1], target)
	} else {
		return json.NewDecoder(resp.Body).Decode(target)
	}
}

func downloadDemandPrices (ec2 *Ec2) error {
	var data map[string]interface{}
	if err := getJson(ec2PricesURL, &data, false); err != nil {
		return err
	}
	serverTypes, _ 			:= data["products"].(map[string]interface{})

	r_mem 	:= regexp.MustCompile(`(\d+)(?:(\.\d+))*\s+GiB`)
	r_disk 	:= regexp.MustCompile(`(\d)\s+x\s+(\d+)(?:\s+(SSD|HDD))*`)

	for server, serverSpecs := range serverTypes {
		serverSpecs, ok := serverSpecs.(map[string]interface{})
		if !ok {
			return errors.New("Type assertion failed on Server Specs")
		}

		// make sure this is actually a EC2 server JSON object
		family, ok := serverSpecs["productFamily"].(string)
		if !ok || family != "Compute Instance" {
			continue
		}

		serverAttributes, ok := serverSpecs["attributes"].(map[string]interface{})
		if ! ok {
			return errors.New("Type assertion failed on attributes")
		}

		// just process all but unknown OS
		os, ok := serverAttributes["operatingSystem"].(string)
		if !ok || os == "NA"  {
			continue
		}

		// drop anything thats bring your own license
		license, ok := serverAttributes["licenseModel"].(string)
		if !ok || (license == "Bring your own license") {
			continue
		}

		// drop anything having pre-installed software
		software, ok := serverAttributes["preInstalledSw"].(string)
		if !ok || (software != "" && software != "NA") {
			continue
		}

		// drop anything that is not shared hosting
		tenancy, ok := serverAttributes["tenancy"].(string)
		if !ok || (tenancy != "Shared") {
			continue
		}

		mem := r_mem.FindStringSubmatch(serverAttributes["memory"].(string))

		var i Instance

		i.Name, ok = serverAttributes["instanceType"].(string)
		i.RegionName, ok = serverAttributes["location"].(string)
		i.RegionCode = ec2RegionMap[i.RegionName]
		i.Sku = server
		i.Specs.Cpu, _  = strconv.Atoi(serverAttributes["vcpu"].(string))
		i.Specs.CpuClock, ok = serverAttributes["clockSpeed"].(string)
		i.Specs.NetworkDesc, ok = serverAttributes["networkPerformance"].(string)
		i.Specs.Os = os


		if (len(mem) >= 2) {
			i.Specs.Mem, _ = strconv.ParseFloat(mem[1] + mem[2], 64)
		} else {
			i.Specs.Mem = 0 // basically could not match memory
		}

		// set networkType code based on networkDesc
		switch i.Specs.NetworkDesc {
		case `10 Gigabit`:
			i.Specs.NetworkType = 1
		case `High`:
			i.Specs.NetworkType = 2
		case `Moderate`:
			i.Specs.NetworkType = 3
		default:
			i.Specs.NetworkType = 4
		}

		if serverAttributes["storage"].(string) == "EBS only" {
			i.Specs.DiskSize = 0
			i.Specs.DiskType ="EBS"
		} else {
			disk := r_disk.FindStringSubmatch(serverAttributes["storage"].(string))
			if len(disk) == 4 {
				diskSize, _ := strconv.ParseInt(disk[1], 10, 32)
				numDisks, _ := strconv.ParseInt(disk[2], 10, 32)
				i.Specs.DiskSize = int(diskSize * numDisks)
				if len(disk[3]) < 1 {
					i.Specs.DiskType = "HDD"
				} else {
					i.Specs.DiskType = disk[3]
				}
			}
		}

		// The price JSON structure is somewhat crazy and has a bunch of hardcoded ID's the structure below
		// is used to capture the required fields, and their hardcoded codes - which is then looped through to find what we need.

		prices := [][]string{
			{"demandPrice"									, "OnDemand", ".JRTCKXETXF", ".JRTCKXETXF.6YS6EN2CT7", ""},
			{"reservedPartial1Year"					, "Reserved", ".HU7G6KETJZ", ".HU7G6KETJZ.6YS6EN2CT7", ""},
			{"reservedPartial1YearUpfront"	, "Reserved", ".HU7G6KETJZ", ".HU7G6KETJZ.2TG2D8R56U", ""},
			{"reservedZero1Year"						, "Reserved", ".4NA7Y494T4", ".4NA7Y494T4.6YS6EN2CT7", ""},
			{"reservedFull1YearUpfront"			, "Reserved", ".6QCMYABX3D", ".6QCMYABX3D.2TG2D8R56U", ""},
			{"reservedPartial3Year"					, "Reserved", ".38NPMPTW36", ".38NPMPTW36.6YS6EN2CT7", ""},
			{"reservedPartial3YearUpfront"	, "Reserved", ".38NPMPTW36", ".38NPMPTW36.2TG2D8R56U", ""},
			{"reservedPFull3YearUpFront"		, "Reserved", ".NQ3QZPMQV9", ".NQ3QZPMQV9.2TG2D8R56U", ""},
		}

		for row := range prices {
			object, ok := data["terms"].(map[string]interface{})[prices[row][1]].(map[string]interface{})[i.Sku].(map[string]interface{})
			if ok {
				price, ok := object[i.Sku + prices[row][2]].(map[string]interface{})
				if ok {
					price, ok := price["priceDimensions"].(map[string]interface{})[i.Sku + prices[row][3]].(map[string]interface{})
					if ok {
						prices[row][4], _ = price["pricePerUnit"].(map[string]interface{})["USD"].(string)
					}
				}
			}
		}

		i.DemandPrice, _ 							= strconv.ParseFloat(prices[0][4],64)
		i.Reserve1YPartialPrice, _ 		= strconv.ParseFloat(prices[1][4],64)
		i.Reserve1YPartialUpfront, _ 	= strconv.ParseFloat(prices[2][4],64)
		i.Reserve1YZeroPrice, _  			= strconv.ParseFloat(prices[3][4],64)
		i.Reserve1YFullUpfront, _ 		= strconv.ParseFloat(prices[4][4],64)
		i.Reserve3YPartialPrice, _		= strconv.ParseFloat(prices[5][4],64)
		i.Reserve3YPartialUpfront,_		= strconv.ParseFloat(prices[6][4],64)
		i.Reserve3YFullUpfront,_			= strconv.ParseFloat(prices[7][4],64)


		// set fake spot price which should get over-set
		i.SpotPrice = 999999.9

		ec2.Instance = append(ec2.Instance, i)
	}
	return nil
}

func downloadSpotPrices (ec2 *Ec2) error {

	var data map[string]interface{}
	if err := getJson(ec2SpotPricesURL, &data, true); err != nil {
		return err
	}
	regions, _ := data["config"].(map[string]interface{})["regions"].([]interface {})
	for r := range regions {
		region, _ := regions[r].(map[string]interface {})
		regionCode, _ := region["region"].(string)

		for instanceType := range region["instanceTypes"].([]interface {}) {
			for size := range region["instanceTypes"].([]interface {})[instanceType].(map[string]interface {})["sizes"].([]interface {}) {
				instance, _ := region["instanceTypes"].([]interface {})[instanceType].(map[string]interface {})["sizes"].([]interface {})[size].(map[string]interface{})

				 for osType := range instance["valueColumns"].([]interface {}) {
					 os, _ := instance["valueColumns"].([]interface {})[osType].(map[string]interface{})
					 var i Instance
					 i.Name, _ 				= instance["size"].(string)
					 i.RegionCode   	= ec2RSpotegionMap[regionCode]
					 // Convert Spot OS Names to ones that match the Demand Names!!!
					 switch os["name"].(string) {
					 	case "linux":
							i.Specs.Os = "Linux"
						case "mswin":
							i.Specs.Os = "Windows"
						default:
					 }
					 i.SpotPrice, _ = strconv.ParseFloat(os["prices"].(map[string]interface {})["USD"].(string),64)
					 ec2.Instance = append(ec2.Instance, i)
				 }
			}
		}
	}
	return nil
}

func combinePrices (demand *Ec2, spot *Ec2) error {

	for d := range demand.Instance {
		for s := range spot.Instance {
			if demand.Instance[d].Specs.Os == spot.Instance[s].Specs.Os 		&&
				 demand.Instance[d].RegionCode == spot.Instance[s].RegionCode &&
				 demand.Instance[d].Name == spot.Instance[s].Name 						&&
				 spot.Instance[s].SpotPrice > 0  {
				 demand.Instance[d].SpotPrice = spot.Instance[s].SpotPrice
				 break
			 }
		}
	}
	return nil
}


func readCache(s *Ec2, cacheFile string, maxCache time.Duration, skipDownload bool) error {

	// get homedir
	home, err := homedir.Dir()
	if err != nil {
		return err
	}

	// check for presense of $HOME/.ec2FleetCompare directory, if doesnt exist create it
	if _, err = os.Stat(home + "/" + cacheDir); err != nil {
		if err = os.Mkdir(home + "/" + cacheDir, 0755); err != nil {
			return err
		}
	}

	// get cache files metaData
	metaCache, err := os.Stat(home + "/" + cacheDir + "/" + cacheFile)
	if err != nil {
		return errors.New("Cache Doesnt exist")
	}

	if ! skipDownload && metaCache.ModTime().Before(time.Now().Add(- maxCache)) {
		return errors.New("Cache too old")
	}

	// our cache file is present and not to old!
	b, err := ioutil.ReadFile(home + "/" + cacheDir + "/" + cacheFile)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(b, s); err != nil {
		return err
	}

	return nil
}

func writeCache (b []byte, cacheFile string) error {
	// get homedir
	home, err := homedir.Dir()
	if err != nil {
		return err
	}

	if err = ioutil.WriteFile(home + "/" + cacheDir + "/" + cacheFile, b, 0644); err != nil {
		return err
	}
	return nil
}

/*

getPrices fetches data for both demand/reserve and spot prices.
As spot is much more variable the cache is kept seperatly and updated more often.

Both demand and spot data structures are the same (for ease of reuse) and then combined. This is a little wasteful in terms of memory but really not alot.

*/
func getPrices(s *Ec2, forceDownload bool, ignoreSpot bool, skipDownload bool) error {

	// First get demand and reserve pricing
	if forceDownload || readCache(s, "ec2.cache", (24 * time.Hour), skipDownload) != nil {
			// cache to old download it
			fmt.Println("Price cache to old fetching new data ...")
			if err := downloadDemandPrices (s); err != nil {
				return err
			}

			// write processed response to cache
			b, _ := json.Marshal(s)
			if err := writeCache(b, "ec2.cache"); err != nil {
				return err
			}
	}

	// now get spot pricing if required
	if !ignoreSpot {
		var spot Ec2
		if forceDownload || readCache(&spot, "spot.cache", (30 * time.Minute), skipDownload) != nil {
			if err := downloadSpotPrices(&spot); err != nil {
				return err
			}

			// write processed response to cache
			b, _ := json.Marshal(spot)
			if err := writeCache(b, "spot.cache"); err != nil {
				return err
			}
		}
		// combine demand and spot prices
		if err := combinePrices(s, &spot); err != nil {
			return err
		}
	}

	return nil
}

func roundUp(val float64) int {
    if val > 0 { return int(val+0.999999) }
    return int(val)
}

func doFilter(ec2 Ec2, region string, instanceCount int, minInstanceCount int, minCPU int, minFleetCPU int, minMem int, minFleetMem int, minDisk int, diskType string, minNetworkType int, operatingSystem string, instanceType string, riType string, sort string) FilteredResults {

	var output FilteredResults

	r_region := regexp.MustCompile(`(?i).*` + region + `.*`)
	r_os		 := regexp.MustCompile(`(?i).*` + operatingSystem + `.*`)
	r_type	 := regexp.MustCompile(`(?i).*` + instanceType + `.*`)


	for i := range ec2.Instance {
		if ! r_region.MatchString(ec2.Instance[i].RegionCode) {
			continue
		}
		if instanceType != "ANY" && ! r_type.MatchString(ec2.Instance[i].Name) {
			continue
		}
		if operatingSystem != "ANY" && ! r_os.MatchString(ec2.Instance[i].Specs.Os) {
			continue
		}
		if ec2.Instance[i].Specs.NetworkType > minNetworkType { // smaller NetworkType is faster!
			continue
		}
		if diskType != "ANY" && diskType != ec2.Instance[i].Specs.DiskType {
			continue
		}
		if minDisk > ec2.Instance[i].Specs.DiskSize {
			continue
		}
		if minCPU > ec2.Instance[i].Specs.Cpu   {
			continue
		}
		if minMem > int(ec2.Instance[i].Specs.Mem) {
			continue
		}

		var numServers int
		// sort prices will be whatever the sort prices set * num instances required (biggest to meet either mem or cpu limits)
		if (instanceCount == 1) {
			if (float64(minFleetMem) / ec2.Instance[i].Specs.Mem) > float64(minFleetCPU / ec2.Instance[i].Specs.Cpu) {
				numServers = roundUp(float64(minFleetMem) / float64(ec2.Instance[i].Specs.Mem))
			} else {
				numServers = roundUp(float64(minFleetCPU) / float64(ec2.Instance[i].Specs.Cpu))
			}
			if numServers < 1 {
				numServers = 1
			}
		} else {
			numServers = instanceCount
		}

		if numServers < minInstanceCount {
			continue
		}

		var riPrice, riMonCost float64
		switch riType {
		case `zero1`:
			riPrice = ec2.Instance[i].Reserve1YZeroPrice * float64(numServers)
			riMonCost = 0
		case `partial1`:
			riPrice = ec2.Instance[i].Reserve1YPartialPrice * float64(numServers)
			riMonCost = (ec2.Instance[i].Reserve1YPartialUpfront / 12) * float64(numServers)
		case `partial3`:
			riPrice = ec2.Instance[i].Reserve3YPartialPrice * float64(numServers)
			riMonCost = (ec2.Instance[i].Reserve3YPartialUpfront / 36) * float64(numServers)
		case `full1`:
			riPrice = 0
			riMonCost = (ec2.Instance[i].Reserve1YFullUpfront / 12) * float64(numServers)
		case `full3`:
			riPrice = 0
			riMonCost = (ec2.Instance[i].Reserve1YFullUpfront / 36) * float64(numServers)
		default:
			riPrice = ec2.Instance[i].Reserve1YZeroPrice * float64(numServers)
			riMonCost = 0
		}

		var instance Ec2Filtered
		instance.NumberInstances 	= numServers
		instance.Instance 				= ec2.Instance[i]

		// calculate monthly costs for demand, spot and choosen RI
		instance.TotalPriceDemand = ec2.Instance[i].DemandPrice * float64(numServers) * 24 * 30
		instance.TotalPriceSpot   = ec2.Instance[i].SpotPrice * float64(numServers) * 24 * 30
		instance.TotalPriceRI     = (riPrice * 24 * 30) + riMonCost

		if instance.TotalPriceRI == 0 {
			instance.TotalPriceRI = 999999999.999999
		}

		switch sort {
			case `demand`:
				instance.SortPrice = instance.TotalPriceDemand
			case `spot`:
				instance.SortPrice = instance.TotalPriceSpot
			case `ri`:
				instance.SortPrice = instance.TotalPriceRI
			default:
				instance.SortPrice = instance.TotalPriceDemand
		}

		output = append(output, instance)
	}

	return output
}

func doDisplay (output FilteredResults, outputSize int) {

	sort.Sort(output)

	var data [][]string
	i := 1
	for _, s := range output {

		if (i > outputSize) {
			break
		}

		spotSaving	:= (((s.TotalPriceDemand - s.TotalPriceSpot)/s.TotalPriceDemand) * 100)

		demandString := "$" + strconv.FormatFloat(s.Instance.DemandPrice * float64(s.NumberInstances), 'f', 2, 64)
		spotString := "$" + strconv.FormatFloat(s.Instance.SpotPrice * float64(s.NumberInstances), 'f', 2, 64)

		if s.NumberInstances > 1 {
			demandString = demandString + " ($" + strconv.FormatFloat(s.Instance.DemandPrice, 'f', 2, 64) + " ea)"
			spotString = spotString + " ($" + strconv.FormatFloat(s.Instance.SpotPrice, 'f', 2, 64) + " ea)"
		}

		result := []string{
			strconv.FormatInt(int64(s.NumberInstances), 10),
			s.Instance.Name,
			strconv.FormatInt(int64(s.Instance.Specs.Cpu), 10),
			s.Instance.Specs.CpuClock,
			strconv.FormatFloat(s.Instance.Specs.Mem, 'f', 1, 64),
			s.Instance.Specs.NetworkDesc,
			s.Instance.Specs.DiskType,
			strconv.FormatInt(int64(s.Instance.Specs.DiskSize), 10) + " GB",
			demandString,
			spotString,
			strconv.FormatFloat(spotSaving, 'f', 0, 64) + "%",
			"$" +   humanize.Comma(int64(s.TotalPriceDemand)),
			"$" + 	humanize.Comma(int64(s.TotalPriceRI)),
			"$" + 	humanize.Comma(int64(s.TotalPriceSpot)),
		}
		if s.Instance.SpotPrice == 999999.9 {
			result[9] = "N/A"
			result[10] = "N/A"
			result[13] = "N/A"
		}
		if s.Instance.Specs.DiskSize == 0 {
			result[6] = "N/A"
			result[7] = "N/A"
		}
		if s.TotalPriceRI == 999999999.999999 {
			result[12] = "N/A"
		}

		data = append(data, result)
		i++
	}
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"# Inst", "Type", "VCPU", "VCPU Freq", "Mem", "Network", "IS Type", "IS Size", "Demand/Hour", "Spot/Hour", "Spot Sav", "Demand/Mon", "RI/Mon", "Spot/Mon"})
	table.SetBorder(true)                                // Set Border to false
	table.AppendBulk(data)                                // Add Bulk Data
	table.Render()
}




func main() {

	app := cli.NewApp()
	app.Name = "EC2 Instance fleet Compare"
	app.Usage = "Use this app to find the cheapest price for a single or set of EC2 instances given your CPU, memory or network requirements. \n\tGiven a minimum or maximum fleet size and the required resources across the fleet this app will find the cheapest EC2 instances that will fulfil your requirements."
	app.Version = "1.0.0"

	var minNetwork, region, diskType, operatingSystem, sort, instanceType, riType string
	var instanceCount,  minInstanceCount, minCPU, minDisk, minFleetCPU, minMem, minFleetMem, outputSize int
	var forceDownload, ignoreSpot, skipDownload bool
	app.Flags = []cli.Flag{
		cli.IntFlag{
			Name:        "num, n",
			Value:       1,
			Usage:       "Number of instances required in fleet - leave at default unless you have specfic requirements for X instances",
			Destination: &instanceCount,
		},
		cli.IntFlag{
			Name:        "min, mn",
			Value:       1,
			Usage:       "Minimum number of instances required in fleet",
			Destination: &minInstanceCount,
		},
		cli.StringFlag{
			Name:        "region, r",
			Value:       "us-east-1",
			Usage:       "The EC2 region to perform price checks on",
			Destination: &region,
		},
		cli.StringFlag{
			Name:        "instance, i",
			Value:       "any",
			Usage:       "EC2 instance type. partial matching is supported i.e c4, m4, c4.large, xl etc",
			Destination: &instanceType,
		},
		cli.IntFlag{
			Name:        "cpu, c",
			Value:       2,
			Usage:       "Minimum CPU cores required per instance",
			Destination: &minCPU,
		},
		cli.IntFlag{
			Name:        "mem, m",
			Value:       2,
			Usage:       "Minimum memoy (in GiB) required per instance",
			Destination: &minMem,
		},
		cli.IntFlag{
			Name:        "fleetcpu, fc",
			Value:       2,
			Usage:       "Minimum CPU virtual cores required across fleet",
			Destination: &minFleetCPU,
		},
		cli.IntFlag{
			Name:        "fleetmem, fm",
			Value:       2,
			Usage:       "Minimum memoy (in GiB) required across fleet",
			Destination: &minFleetMem,
		},
		cli.StringFlag{
			Name:        "network, nw",
			Value:       "low",
			Usage:       "Minimum network speed required per instance, options: low, medium, high, gbit",
			Destination: &minNetwork,
		},
		cli.IntFlag{
			Name:        "disk, d",
			Value:       0,
			Usage:       "Minimum instance store disk space required (in GiB) per instance",
			Destination: &minDisk,
		},
		cli.StringFlag{
			Name:        "diskType, dt",
			Value:       "any",
			Usage:       "Type of instance store disk required, options: any, hdd, ssd",
			Destination: &diskType,
		},
		cli.StringFlag{
			Name:        "operatingSystem, os",
			Value:       "linux",
			Usage:       "Type of OS required, options: any, linux, windows, rhel, suse",
			Destination: &operatingSystem,
		},
		cli.StringFlag{
			Name:        "sort, s",
			Value:       "demand",
			Usage:       "Sort choice (always low to high), options: demand, spot, ri",
			Destination: &sort,
		},
		cli.BoolFlag{
			Name:        "force, f",
			Usage:       "Force download of latest version of AWS EC2 pricing file",
			Destination: &forceDownload,
		},
		cli.BoolFlag{
			Name:        "skip",
			Usage:       "Skip download of pricing even if cache is old, good for offline use",
			Destination: &skipDownload,
		},
		cli.IntFlag{
			Name:        "outputSize, o",
			Value:       20,
			Usage:       "Max number of output lines",
			Destination: &outputSize,
		},
		cli.StringFlag{
			Name:        "riType, ri",
			Value:       "partial1",
			Usage:       "Type of RI type to display, options: zero1, partial1, partial3, full1, full3",
			Destination: &riType,
		},
	}
	app.Action = func(c *cli.Context) error {
			var prices Ec2
			err := getPrices(&prices, forceDownload, ignoreSpot, skipDownload)
			if err != nil {
				printError(err.Error())
				return err
			}

			minNetworkType := networkMap[minNetwork]
			diskType 				= strings.ToUpper(diskType)
			operatingSystem = strings.ToUpper(operatingSystem)
			instanceType    = strings.ToUpper(instanceType)

			filtered := doFilter(prices, region, instanceCount, minInstanceCount, minCPU, minFleetCPU, minMem, minFleetMem, minDisk, diskType, minNetworkType, operatingSystem, instanceType, riType, sort)
			doDisplay(filtered, outputSize)
			return nil
	}
	app.Run(os.Args)
}
