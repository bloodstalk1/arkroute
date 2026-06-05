import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

const platform = process.platform;
const arch = process.arch;
const platformKey = `${platform}-${arch}`;

try {
  console.log(`Building frontend & Go binary...`);
  execSync("npm run build", { stdio: "inherit" });

  const binaryName = platform === "win32" ? "arkroute.exe" : "arkroute";
  let sourceBinary = path.join("dist", binaryName);
  if (!fs.existsSync(sourceBinary)) {
    const fallbackName = platform === "win32" ? "arkroute" : "arkroute.exe";
    const fallbackBinary = path.join("dist", fallbackName);
    if (fs.existsSync(fallbackBinary)) {
      console.log(`Expected source binary '${sourceBinary}' not found, falling back to found binary '${fallbackBinary}'`);
      sourceBinary = fallbackBinary;
    } else {
      throw new Error(`Could not find compiled binary in dist/ (tried both '${sourceBinary}' and '${fallbackBinary}')`);
    }
  }
  const targetDir = path.join("npm", "platform", platformKey, "bin");
  const targetBinary = path.join(targetDir, binaryName);

  console.log(`Copying binary to local platform package: ${targetBinary}`);
  fs.mkdirSync(targetDir, { recursive: true });
  fs.copyFileSync(sourceBinary, targetBinary);
  fs.chmodSync(targetBinary, 0o755);

  console.log(`Installing platform package globally: ./npm/platform/${platformKey}`);
  execSync(`npm install -g ./npm/platform/${platformKey}`, { stdio: "inherit" });

  console.log(`Installing main package globally: ./npm/arkroute`);
  execSync("npm install -g ./npm/arkroute", { stdio: "inherit" });

  console.log(`\n🎉 Success! 'arkroute' has been built and installed globally via NPM.`);
  console.log(`You can now run 'arkroute setup' from anywhere.`);
} catch (err) {
  console.error(`\n❌ Error during installation:`, err.message);
  process.exit(1);
}
