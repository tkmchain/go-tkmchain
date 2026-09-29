package com.tkmchain.email;

import java.io.BufferedInputStream;
import java.io.BufferedOutputStream;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.InetAddress;
import java.net.InetSocketAddress;
import java.net.Proxy;
import java.net.ServerSocket;
import java.net.Socket;
import java.net.URL;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;

/**
 * A loopback-only HTTP bridge for the Email WebView. Every upstream connection
 * is opened through the supplied SOCKS5 proxy; there is deliberately no direct
 * networking fallback.
 */
final class TorWebViewProxy implements AutoCloseable {
    private static final int MAX_BODY = 16 << 20;
    private static final int MAX_HEADERS = 16 << 10;

    private final String remoteBase;
    private final int socksPort;
    private ServerSocket server;
    private Thread acceptThread;
    private volatile boolean closed;
    private String localOrigin;

    TorWebViewProxy(String remoteBase, int socksPort) {
        this.remoteBase = remoteBase.endsWith("/") ? remoteBase : remoteBase + "/";
        this.socksPort = socksPort;
    }

    void start() throws IOException {
        server = new ServerSocket(0, 16, InetAddress.getByName("127.0.0.1"));
        localOrigin = "http://localhost:" + server.getLocalPort();
        acceptThread = new Thread(() -> {
            while (!closed) {
                try {
                    Socket socket = server.accept();
                    Thread worker = new Thread(() -> handle(socket), "tkm-email-http");
                    worker.setDaemon(true);
                    worker.start();
                } catch (IOException ignored) {
                    if (!closed) break;
                }
            }
        }, "tkm-email-proxy");
        acceptThread.setDaemon(true);
        acceptThread.start();
    }

    String url() {
        return localOrigin + "/";
    }

    String localUrl(String pathAndQuery) {
        if (pathAndQuery == null || pathAndQuery.isEmpty() || !pathAndQuery.startsWith("/")) {
            pathAndQuery = "/";
        }
        return localOrigin + pathAndQuery;
    }

    @Override
    public void close() {
        closed = true;
        if (server != null) {
            try {
                server.close();
            } catch (IOException ignored) {
            }
        }
        if (acceptThread != null) acceptThread.interrupt();
    }

    private void handle(Socket socket) {
        try (Socket client = socket) {
            client.setSoTimeout(120_000);
            BufferedInputStream input = new BufferedInputStream(client.getInputStream());
            BufferedOutputStream output = new BufferedOutputStream(client.getOutputStream());
            String requestLine = readLine(input, MAX_HEADERS);
            if (requestLine == null || requestLine.isEmpty()) return;
            String[] parts = requestLine.split(" ", 3);
            if (parts.length != 3 || !"HTTP/1.1".equalsIgnoreCase(parts[2])) {
                writeError(output, 400, "Bad Request");
                return;
            }
            String method = parts[0].toUpperCase(Locale.ROOT);
            if (!(method.equals("GET") || method.equals("HEAD") || method.equals("POST")
                    || method.equals("PUT") || method.equals("PATCH") || method.equals("DELETE")
                    || method.equals("OPTIONS"))) {
                writeError(output, 405, "Method Not Allowed");
                return;
            }
            Map<String, String> headers = new LinkedHashMap<>();
            int headerBytes = requestLine.length();
            while (true) {
                String line = readLine(input, MAX_HEADERS);
                if (line == null) {
                    writeError(output, 400, "Bad Request");
                    return;
                }
                headerBytes += line.length();
                if (headerBytes > MAX_HEADERS) {
                    writeError(output, 431, "Request Header Fields Too Large");
                    return;
                }
                if (line.isEmpty()) break;
                int colon = line.indexOf(':');
                if (colon <= 0) {
                    writeError(output, 400, "Bad Request");
                    return;
                }
                headers.put(line.substring(0, colon).trim().toLowerCase(Locale.ROOT),
                        line.substring(colon + 1).trim());
            }
            int length = 0;
            if (headers.containsKey("content-length")) {
                try {
                    length = Integer.parseInt(headers.get("content-length"));
                } catch (NumberFormatException e) {
                    writeError(output, 400, "Bad Request");
                    return;
                }
                if (length < 0 || length > MAX_BODY) {
                    writeError(output, 413, "Payload Too Large");
                    return;
                }
            }
            byte[] body = readRequestBody(input, length);
            URL upstream = upstreamURL(parts[1]);
            Proxy socks = new Proxy(Proxy.Type.SOCKS,
                    InetSocketAddress.createUnresolved("127.0.0.1", socksPort));
            HttpURLConnection connection = (HttpURLConnection) upstream.openConnection(socks);
            connection.setConnectTimeout(20_000);
            connection.setReadTimeout(120_000);
            connection.setInstanceFollowRedirects(false);
            connection.setRequestMethod(method);
            for (Map.Entry<String, String> entry : headers.entrySet()) {
                String key = entry.getKey();
                if (key.equals("host") || key.equals("connection") || key.equals("content-length")
                        || key.equals("accept-encoding")) continue;
                String value = entry.getValue();
                if (key.equals("origin") && value.startsWith(localOrigin)) {
                    value = originOf(remoteBase);
                } else if (key.equals("referer") && value.startsWith(localOrigin)) {
                    value = originOf(remoteBase) + value.substring(localOrigin.length());
                }
                connection.setRequestProperty(key, value);
            }
            connection.setRequestProperty("Accept-Encoding", "identity");
            if (length > 0 && !method.equals("GET") && !method.equals("HEAD")) {
                connection.setDoOutput(true);
                connection.setFixedLengthStreamingMode(length);
                try (OutputStream requestBody = connection.getOutputStream()) {
                    requestBody.write(body);
                }
            }
            int status = connection.getResponseCode();
            InputStream responseStream = status >= 400 ? connection.getErrorStream() : connection.getInputStream();
            byte[] responseBody = responseStream == null ? new byte[0] : readBody(responseStream, MAX_BODY);
            String contentType = connection.getHeaderField("Content-Type");
            if (contentType != null && (contentType.contains("text/") || contentType.contains("javascript")
                    || contentType.contains("json"))) {
                responseBody = replace(responseBody, remoteBase, localOrigin + "/");
                responseBody = replace(responseBody, originOf(remoteBase), localOrigin);
            }
            writeResponse(output, status, connection.getResponseMessage(), connection, responseBody);
            connection.disconnect();
        } catch (Exception e) {
            try {
                OutputStream output = socket.getOutputStream();
                writeError(output, 502, "Tor upstream unavailable");
            } catch (IOException ignored) {
            }
        }
    }

    private URL upstreamURL(String target) throws IOException {
        if (!target.startsWith("/")) target = "/";
        URL base = new URL(remoteBase);
        String basePath = base.getPath();
        if (!basePath.endsWith("/")) basePath += "/";
        String targetPath = target;
        String query = null;
        int queryStart = target.indexOf('?');
        if (queryStart >= 0) {
            targetPath = target.substring(0, queryStart);
            query = target.substring(queryStart + 1);
        } else if ("/".equals(targetPath)) {
            // The launcher URL selects the wallet's email tab. Preserve that
            // query only for the initial document, not for assets or RPC.
            query = base.getQuery();
        }
        String path = basePath + targetPath.substring(1);
        if (query != null && !query.isEmpty()) path += "?" + query;
        return new URL(base.getProtocol(), base.getHost(), base.getPort(), path);
    }

    private static String originOf(String raw) {
        try {
            URL url = new URL(raw);
            return url.getProtocol() + "://" + url.getAuthority();
        } catch (Exception e) {
            return raw;
        }
    }

    private void writeResponse(OutputStream output, int status, String message,
                               HttpURLConnection connection, byte[] body) throws IOException {
        String reason = message == null ? "" : message.replace('\r', ' ').replace('\n', ' ');
        StringBuilder response = new StringBuilder("HTTP/1.1 ").append(status).append(' ').append(reason).append("\r\n");
        Map<String, List<String>> fields = connection.getHeaderFields();
        if (fields != null) {
            for (Map.Entry<String, List<String>> entry : fields.entrySet()) {
                String key = entry.getKey();
                if (key == null || key.equalsIgnoreCase("Content-Length") || key.equalsIgnoreCase("Content-Encoding")
                        || key.equalsIgnoreCase("Transfer-Encoding") || key.equalsIgnoreCase("Connection")) continue;
                for (String value : entry.getValue()) {
                    if (key.equalsIgnoreCase("Location")) value = localizeLocation(value);
                    response.append(key).append(": ").append(value).append("\r\n");
                }
            }
        }
        response.append("Content-Length: ").append(body.length).append("\r\nConnection: close\r\n\r\n");
        output.write(response.toString().getBytes("UTF-8"));
        output.write(body);
        output.flush();
    }

    private String localizeLocation(String raw) {
        if (raw == null || !raw.startsWith(originOf(remoteBase))) return raw;
        return localOrigin + raw.substring(originOf(remoteBase).length());
    }

    private static void writeError(OutputStream output, int status, String message) throws IOException {
        byte[] body = message.getBytes("UTF-8");
        String head = "HTTP/1.1 " + status + " " + message + "\r\nContent-Type: text/plain\r\nContent-Length: "
                + body.length + "\r\nConnection: close\r\n\r\n";
        output.write(head.getBytes("UTF-8"));
        output.write(body);
        output.flush();
    }

    private static String readLine(InputStream input, int max) throws IOException {
        ByteArrayOutputStream line = new ByteArrayOutputStream();
        int current;
        while ((current = input.read()) != -1) {
            if (current == '\n') break;
            if (current == '\r') continue;
            if (line.size() >= max) throw new IOException("line too long");
            line.write(current);
        }
        if (current == -1 && line.size() == 0) return null;
        return line.toString("ISO-8859-1");
    }

    private static byte[] readRequestBody(InputStream input, int length) throws IOException {
        byte[] body = new byte[length];
        int offset = 0;
        while (offset < length) {
            int count = input.read(body, offset, length - offset);
            if (count < 0) throw new IOException("truncated request");
            offset += count;
        }
        return body;
    }

    private static byte[] readBody(InputStream input, int limit) throws IOException {
        ByteArrayOutputStream output = new ByteArrayOutputStream();
        byte[] buffer = new byte[8192];
        int count;
        while ((count = input.read(buffer)) != -1) {
            if (output.size() + count > limit) throw new IOException("response too large");
            output.write(buffer, 0, count);
        }
        return output.toByteArray();
    }

    private static byte[] replace(byte[] input, String oldValue, String newValue) {
        try {
            return new String(input, "UTF-8").replace(oldValue, newValue).getBytes("UTF-8");
        } catch (java.io.UnsupportedEncodingException e) {
            return input;
        }
    }
}
