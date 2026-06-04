import type { KnipConfig } from "knip";

const config: KnipConfig = {
  // Ignore build output — not source files.
  ignore: ["dist/**"],
  // BleephubAgent and BleephubLabel are referenced via embedding inside
  // BleephubSession (field: agent: BleephubAgent | null). Knip doesn't
  // trace intra-file type references as "used exports", so suppress these.
  ignoreDependencies: [],
  ignoreExportsUsedInFile: true,
};

export default config;
