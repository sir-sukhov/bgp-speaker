package speaker

type Config struct {
	AnycastIP string `yaml:"anycast_ip"`
	ASN       uint32 `yaml:"asn"`
	Neighbors []struct {
		Address      string `yaml:"address"`
		LocalAddress string `yaml:"local_address"`
		ASN          uint32 `yaml:"asn"`
	} `yaml:"neighbors"`
}
