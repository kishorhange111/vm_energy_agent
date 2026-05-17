package config

import (
	"os"
	"strconv"
)

// Config holds all agent settings, loaded once from environment variables.
type Config struct {
	Port         string
	InstanceName string
	VMName       string
	// Power model coefficients (Strategy pattern  swappable without recompile)
	CPUCoeff     float64
	MemoryCoeff  float64
	DiskCoeff    float64
	NetworkCoeff float64
}

func Load() Config {
	return Config{
		Port:         getEnv("AGENT_PORT", "8080"),
		InstanceName: getEnv("AGENT_INSTANCE_NAME", "instance-01"),
		VMName:       getEnv("AGENT_VM_NAME", "vm-linux"),
		CPUCoeff:     getEnvFloat("AGENT_CPU_COEFFICIENT", 0.5),
		MemoryCoeff:  getEnvFloat("AGENT_MEMORY_COEFFICIENT", 0.3),
		DiskCoeff:    getEnvFloat("AGENT_DISK_COEFFICIENT", 0.1),
		NetworkCoeff: getEnvFloat("AGENT_NETWORK_COEFFICIENT", 0.1),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}
