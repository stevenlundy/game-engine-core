import type { Config } from "jest";

const config: Config = {
  preset: "ts-jest",
  testEnvironment: "node",
  testMatch: ["**/src/**/*.test.ts"],
  moduleFileExtensions: ["ts", "tsx", "js", "jsx", "json", "node"],
  moduleNameMapper: {
    // Allow TypeScript source files imported with .js extension (Node16 module resolution)
    "^(\\.{1,2}/.*)\\.js$": "$1",
  },
};

export default config;
