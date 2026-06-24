#!/usr/bin/env node

const fs = require("fs/promises");
const path = require("path");
const readline = require("readline");

const root = path.resolve(process.env.MD_MCP_ROOT || path.join(__dirname, "..", "md_mcp_data"));

function send(message) {
  process.stdout.write(`${JSON.stringify(message)}\n`);
}

function result(id, value) {
  send({ jsonrpc: "2.0", id, result: value });
}

function error(id, code, message) {
  send({ jsonrpc: "2.0", id, error: { code, message } });
}

function resolveMarkdownPath(filePath) {
  if (!filePath || typeof filePath !== "string") {
    throw new Error("path is required");
  }
  if (!filePath.endsWith(".md")) {
    throw new Error("only .md files are allowed");
  }

  const resolved = path.resolve(root, filePath);
  const relative = path.relative(root, resolved);
  if (relative.startsWith("..") || path.isAbsolute(relative)) {
    throw new Error("path must stay inside MD_MCP_ROOT");
  }
  return resolved;
}

const tools = [
  {
    name: "read_md",
    description: "Read a Markdown file under MD_MCP_ROOT.",
    inputSchema: {
      type: "object",
      required: ["path"],
      properties: {
        path: {
          type: "string",
          description: "Markdown path relative to MD_MCP_ROOT, for example notes.md",
        },
      },
    },
  },
  {
    name: "write_md",
    description: "Create or overwrite a Markdown file under MD_MCP_ROOT.",
    inputSchema: {
      type: "object",
      required: ["path", "content"],
      properties: {
        path: {
          type: "string",
          description: "Markdown path relative to MD_MCP_ROOT, for example notes.md",
        },
        content: {
          type: "string",
          description: "Full Markdown content to write.",
        },
      },
    },
  },
];

async function handleToolCall(params) {
  const name = params && params.name;
  const args = (params && params.arguments) || {};

  if (name === "read_md") {
    const target = resolveMarkdownPath(args.path);
    const content = await fs.readFile(target, "utf8");
    return {
      content: [{ type: "text", text: content }],
    };
  }

  if (name === "write_md") {
    if (typeof args.content !== "string") {
      throw new Error("content is required");
    }
    const target = resolveMarkdownPath(args.path);
    await fs.mkdir(path.dirname(target), { recursive: true });
    await fs.writeFile(target, args.content, "utf8");
    return {
      content: [{ type: "text", text: `wrote ${path.relative(root, target)}` }],
    };
  }

  throw new Error(`unknown tool: ${name}`);
}

async function handle(request) {
  const id = request.id;
  try {
    switch (request.method) {
      case "initialize":
        result(id, {
          protocolVersion: "2024-11-05",
          capabilities: {
            tools: { listChanged: false },
          },
          serverInfo: {
            name: "md-mcp",
            version: "0.1.0",
          },
        });
        return;
      case "initialized":
        return;
      case "tools/list":
        result(id, { tools });
        return;
      case "tools/call":
        result(id, await handleToolCall(request.params || {}));
        return;
      default:
        error(id, -32601, `method not found: ${request.method}`);
    }
  } catch (err) {
    error(id, -32000, err.message || String(err));
  }
}

async function main() {
  await fs.mkdir(root, { recursive: true });

  const rl = readline.createInterface({
    input: process.stdin,
    crlfDelay: Infinity,
  });

  for await (const line of rl) {
    if (!line.trim()) {
      continue;
    }
    let request;
    try {
      request = JSON.parse(line);
    } catch (err) {
      error(null, -32700, "parse error");
      continue;
    }
    await handle(request);
  }
}

main().catch((err) => {
  process.stderr.write(`${err.stack || err}\n`);
  process.exit(1);
});
