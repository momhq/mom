import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";

export interface DetectedStack {
  name: string;
  confidence: "high" | "medium";
  detectedBy: string;
}

export interface SuggestedSpecialist {
  name: string;
  description: string;
  domain: string;
  reason: string;
}

interface StackDetector {
  name: string;
  detect: (projectDir: string) => boolean;
  detectedBy: string;
  specialists: SuggestedSpecialist[];
}

function fileExists(projectDir: string, ...paths: string[]): boolean {
  return paths.some((p) => existsSync(resolve(projectDir, p)));
}

function depExists(projectDir: string, dep: string): boolean {
  const pkgPath = resolve(projectDir, "package.json");
  if (!existsSync(pkgPath)) return false;
  try {
    const pkg = JSON.parse(readFileSync(pkgPath, "utf-8"));
    const allDeps = {
      ...pkg.dependencies,
      ...pkg.devDependencies,
    };
    return dep in allDeps;
  } catch {
    return false;
  }
}

function globExists(projectDir: string, pattern: string): boolean {
  // Simple check for config files with known extensions
  const extensions = [".js", ".ts", ".mjs", ".cjs", ".json"];
  const base = pattern.replace(".*", "");
  return extensions.some((ext) =>
    existsSync(resolve(projectDir, base + ext))
  );
}

const detectors: StackDetector[] = [
  // --- Frameworks ---
  {
    name: "Next.js",
    detect: (dir) =>
      fileExists(dir, "next.config.js", "next.config.ts", "next.config.mjs"),
    detectedBy: "next.config.*",
    specialists: [
      {
        name: "frontend-nextjs-specialist",
        description: "Next.js App Router, SSR/SSG, middleware, and API routes",
        domain: "dev",
        reason: "Next.js project detected",
      },
    ],
  },
  {
    name: "Vite",
    detect: (dir) =>
      fileExists(dir, "vite.config.js", "vite.config.ts", "vite.config.mjs"),
    detectedBy: "vite.config.*",
    specialists: [
      {
        name: "frontend-vite-specialist",
        description: "Vite bundling, plugin configuration, and HMR",
        domain: "dev",
        reason: "Vite project detected",
      },
    ],
  },
  {
    name: "React",
    detect: (dir) => depExists(dir, "react"),
    detectedBy: "react in dependencies",
    specialists: [],  // covered by framework specialist (Next/Vite)
  },
  {
    name: "Vue",
    detect: (dir) => depExists(dir, "vue"),
    detectedBy: "vue in dependencies",
    specialists: [
      {
        name: "frontend-vue-specialist",
        description: "Vue 3 Composition API, Pinia, Vue Router",
        domain: "dev",
        reason: "Vue dependency detected",
      },
    ],
  },

  // --- Mobile ---
  {
    name: "Capacitor",
    detect: (dir) =>
      fileExists(dir, "capacitor.config.ts", "capacitor.config.json"),
    detectedBy: "capacitor.config.*",
    specialists: [
      {
        name: "mobile-capacitor-specialist",
        description: "Capacitor native bridging, plugins, iOS/Android builds",
        domain: "dev",
        reason: "Capacitor config detected",
      },
    ],
  },
  {
    name: "React Native",
    detect: (dir) => depExists(dir, "react-native"),
    detectedBy: "react-native in dependencies",
    specialists: [
      {
        name: "mobile-rn-specialist",
        description: "React Native CLI/Expo, native modules, platform-specific code",
        domain: "dev",
        reason: "React Native dependency detected",
      },
    ],
  },
  {
    name: "Flutter",
    detect: (dir) => fileExists(dir, "pubspec.yaml"),
    detectedBy: "pubspec.yaml",
    specialists: [
      {
        name: "mobile-flutter-specialist",
        description: "Flutter/Dart widgets, state management, platform channels",
        domain: "dev",
        reason: "pubspec.yaml detected",
      },
    ],
  },
  {
    name: "iOS Native",
    detect: (dir) => fileExists(dir, "Podfile") && !depExists(dir, "react-native"),
    detectedBy: "Podfile (no React Native)",
    specialists: [
      {
        name: "ios-native-specialist",
        description: "Swift/SwiftUI, UIKit, Xcode project configuration",
        domain: "dev",
        reason: "Podfile detected without React Native",
      },
    ],
  },
  {
    name: "Android Native",
    detect: (dir) =>
      fileExists(dir, "android/build.gradle", "android/build.gradle.kts") &&
      !depExists(dir, "react-native"),
    detectedBy: "android/build.gradle",
    specialists: [
      {
        name: "android-native-specialist",
        description: "Kotlin/Java, Jetpack Compose, Gradle configuration",
        domain: "dev",
        reason: "Android build.gradle detected",
      },
    ],
  },

  // --- Backend / DB ---
  {
    name: "Supabase",
    detect: (dir) =>
      depExists(dir, "@supabase/supabase-js") ||
      fileExists(dir, "supabase/config.toml"),
    detectedBy: "@supabase/supabase-js or supabase/config.toml",
    specialists: [
      {
        name: "backend-supabase-specialist",
        description: "Supabase Auth, RLS policies, Edge Functions, Realtime",
        domain: "dev",
        reason: "Supabase detected",
      },
    ],
  },
  {
    name: "Prisma",
    detect: (dir) => fileExists(dir, "prisma/schema.prisma"),
    detectedBy: "prisma/schema.prisma",
    specialists: [
      {
        name: "prisma-schema-specialist",
        description: "Prisma schema design, migrations, query optimization",
        domain: "dev",
        reason: "Prisma schema detected",
      },
    ],
  },
  {
    name: "Python",
    detect: (dir) =>
      fileExists(dir, "requirements.txt", "pyproject.toml", "setup.py"),
    detectedBy: "requirements.txt or pyproject.toml",
    specialists: [
      {
        name: "backend-python-specialist",
        description: "Python backend (FastAPI/Django/Flask), async patterns",
        domain: "dev",
        reason: "Python project detected",
      },
    ],
  },
  {
    name: "Go",
    detect: (dir) => fileExists(dir, "go.mod"),
    detectedBy: "go.mod",
    specialists: [
      {
        name: "backend-go-specialist",
        description: "Go modules, concurrency patterns, standard library",
        domain: "dev",
        reason: "go.mod detected",
      },
    ],
  },
  {
    name: "Rust",
    detect: (dir) => fileExists(dir, "Cargo.toml"),
    detectedBy: "Cargo.toml",
    specialists: [
      {
        name: "backend-rust-specialist",
        description: "Rust ownership, async runtime, crate ecosystem",
        domain: "dev",
        reason: "Cargo.toml detected",
      },
    ],
  },

  // --- Styling ---
  {
    name: "Tailwind CSS",
    detect: (dir) =>
      fileExists(
        dir,
        "tailwind.config.js",
        "tailwind.config.ts",
        "tailwind.config.mjs"
      ) || depExists(dir, "tailwindcss"),
    detectedBy: "tailwind.config.* or tailwindcss dependency",
    specialists: [], // standard enough, no specialist needed
  },
  {
    name: "shadcn/ui",
    detect: (dir) =>
      fileExists(dir, "components.json") && depExists(dir, "tailwindcss"),
    detectedBy: "components.json + tailwindcss",
    specialists: [], // handled by frontend specialist
  },

  // --- Infra ---
  {
    name: "Docker",
    detect: (dir) =>
      fileExists(dir, "Dockerfile", "docker-compose.yml", "docker-compose.yaml"),
    detectedBy: "Dockerfile or docker-compose.yml",
    specialists: [
      {
        name: "infra-docker-specialist",
        description: "Dockerfile optimization, compose orchestration, multi-stage builds",
        domain: "dev",
        reason: "Docker config detected",
      },
    ],
  },
  {
    name: "GitHub Actions",
    detect: (dir) => fileExists(dir, ".github/workflows"),
    detectedBy: ".github/workflows/",
    specialists: [
      {
        name: "ci-github-actions-specialist",
        description: "GitHub Actions workflows, reusable actions, matrix builds",
        domain: "dev",
        reason: "GitHub Actions workflows detected",
      },
    ],
  },
  {
    name: "Vercel",
    detect: (dir) =>
      fileExists(dir, "vercel.json") || depExists(dir, "vercel"),
    detectedBy: "vercel.json or vercel dependency",
    specialists: [
      {
        name: "deploy-vercel-specialist",
        description: "Vercel deployments, serverless functions, edge config",
        domain: "dev",
        reason: "Vercel config detected",
      },
    ],
  },

  // --- Package managers ---
  {
    name: "pnpm",
    detect: (dir) => fileExists(dir, "pnpm-lock.yaml"),
    detectedBy: "pnpm-lock.yaml",
    specialists: [],
  },
  {
    name: "yarn",
    detect: (dir) => fileExists(dir, "yarn.lock"),
    detectedBy: "yarn.lock",
    specialists: [],
  },
  {
    name: "bun",
    detect: (dir) => fileExists(dir, "bun.lockb", "bun.lock"),
    detectedBy: "bun.lockb",
    specialists: [],
  },

  // --- Other ---
  {
    name: "TypeScript",
    detect: (dir) => fileExists(dir, "tsconfig.json"),
    detectedBy: "tsconfig.json",
    specialists: [],
  },
  {
    name: "Monorepo (Turborepo)",
    detect: (dir) => fileExists(dir, "turbo.json"),
    detectedBy: "turbo.json",
    specialists: [],
  },
  {
    name: "Monorepo (Nx)",
    detect: (dir) => fileExists(dir, "nx.json"),
    detectedBy: "nx.json",
    specialists: [],
  },
];

export function detectStack(projectDir: string): DetectedStack[] {
  const detected: DetectedStack[] = [];

  for (const detector of detectors) {
    if (detector.detect(projectDir)) {
      detected.push({
        name: detector.name,
        confidence: "high",
        detectedBy: detector.detectedBy,
      });
    }
  }

  return detected;
}

export function suggestSpecialists(projectDir: string): SuggestedSpecialist[] {
  const seen = new Set<string>();
  const suggestions: SuggestedSpecialist[] = [];

  for (const detector of detectors) {
    if (detector.detect(projectDir)) {
      for (const specialist of detector.specialists) {
        if (!seen.has(specialist.name)) {
          seen.add(specialist.name);
          suggestions.push(specialist);
        }
      }
    }
  }

  return suggestions;
}

export function detectPackageManager(projectDir: string): string {
  if (fileExists(projectDir, "pnpm-lock.yaml")) return "pnpm";
  if (fileExists(projectDir, "bun.lockb", "bun.lock")) return "bun";
  if (fileExists(projectDir, "yarn.lock")) return "yarn";
  if (fileExists(projectDir, "package-lock.json")) return "npm";
  return "unknown";
}

export function isMonorepo(projectDir: string): boolean {
  if (fileExists(projectDir, "turbo.json", "nx.json", "lerna.json")) return true;

  const pkgPath = resolve(projectDir, "package.json");
  if (existsSync(pkgPath)) {
    try {
      const pkg = JSON.parse(readFileSync(pkgPath, "utf-8"));
      if (pkg.workspaces) return true;
    } catch {
      // ignore
    }
  }
  return false;
}
