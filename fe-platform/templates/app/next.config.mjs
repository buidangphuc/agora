const nextConfig = {
  reactStrictMode: true,
  transpilePackages: [
    "@fe-platform/ui",
    "@fe-platform/tokens",
    "@fe-platform/api-client",
  ],
  webpack(config) {
    config.resolve.extensionAlias = {
      ".js": [".ts", ".tsx", ".js"],
    };
    return config;
  },
};

export default nextConfig;
