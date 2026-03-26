import type { Config } from "jest";

const config: Config = {
  preset: "ts-jest",
  testEnvironment: "jsdom",
  testMatch: ["**/src/**/*.test.ts"],
  moduleFileExtensions: ["ts", "tsx", "js", "json"],
  setupFiles: ["./jest.setup.ts"],
  transform: {
    "^.+\\.tsx?$": [
      "ts-jest",
      {
        tsconfig: {
          target: "ES2020",
          lib: ["ES2020", "DOM"],
          strict: true,
          esModuleInterop: true,
          moduleResolution: "node",
          skipLibCheck: true,
        },
      },
    ],
  },
};

export default config;
