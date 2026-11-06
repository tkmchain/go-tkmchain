package com.tkmchain.node;

import android.content.ContentResolver;
import android.database.Cursor;
import android.net.Uri;
import android.provider.DocumentsContract;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.HashSet;
import java.util.Set;

/** Imports public proving artifacts only. The Go wallet additionally requires
 * their proving/verifying key hashes to match authenticated network metadata
 * before decoding or using them. No seed or witness enters this importer. */
final class Shield6Parameters {
    private static final String[] FILES = {
            "recipient.r1cs", "recipient.pk", "recipient.vk", "manifest.json",
            "registration.r1cs", "registration.pk", "registration.vk", "registration-manifest.json"
    };
    private static final long RESERVE = 256L << 20;

    interface Progress { void update(String message); }

    static synchronized void install(ContentResolver resolver, Uri tree, File dataDir, Progress progress) throws Exception {
        File stage = new File(dataDir, ".shield6-import-" + System.nanoTime());
        if (!stage.mkdirs()) throw new IOException("Cannot create wallet import directory");
        boolean installed = false;
        try {
            Set<String> copied = new HashSet<>();
            Uri children = DocumentsContract.buildChildDocumentsUriUsingTree(tree,
                    DocumentsContract.getTreeDocumentId(tree));
            try (Cursor cursor = resolver.query(children, new String[]{
                    DocumentsContract.Document.COLUMN_DISPLAY_NAME,
                    DocumentsContract.Document.COLUMN_DOCUMENT_ID}, null, null, null)) {
                if (cursor == null) throw new IOException("Cannot read selected folder");
                while (cursor.moveToNext()) {
                    String name = cursor.getString(0);
                    long limit = limit(name);
                    if (limit == 0) continue;
                    if (!copied.add(name)) throw new IOException("Duplicate wallet file: " + name);
                    Uri document = DocumentsContract.buildDocumentUriUsingTree(tree, cursor.getString(1));
                    try (InputStream input = resolver.openInputStream(document)) {
                        if (input == null) throw new IOException("Cannot open wallet file: " + name);
                        copy(input, new File(stage, name), limit, dataDir);
                    }
                    progress.update("Copying wallet files: " + copied.size() + " of " + FILES.length);
                }
            }
            if (copied.size() != FILES.length) throw new IOException("Select the release folder containing all eight wallet files");
            progress.update("Checking wallet files");
            verify(stage, "recipient", "manifest.json");
            verify(stage, "registration", "registration-manifest.json");
            File target = new File(dataDir, "shield6-recipient");
            File backup = new File(dataDir, ".shield6-backup-" + System.nanoTime());
            boolean hadPrevious = target.exists();
            if (hadPrevious && !target.renameTo(backup)) throw new IOException("Cannot replace current wallet files");
            if (!stage.renameTo(target)) {
                if (hadPrevious && !backup.renameTo(target)) throw new IOException("Import failed; previous files remain in " + backup.getName());
                throw new IOException("Cannot install wallet files");
            }
            installed = true;
            if (hadPrevious) {
                // Only remove files managed by this importer. Preserve any
                // unrelated file that was manually placed in the directory.
                for (String name : FILES) new File(backup, name).delete();
                backup.delete();
            }
        } finally {
            if (!installed) remove(stage);
        }
    }

    private static long limit(String name) {
        for (String allowed : FILES) if (allowed.equals(name)) {
            if (name.endsWith(".json")) return 4096;
            if (name.endsWith(".vk")) return 64L << 10;
            if (name.endsWith(".r1cs")) return 512L << 20;
            return 2L << 30;
        }
        return 0;
    }

    private static void copy(InputStream input, File output, long limit, File dataDir) throws IOException {
        byte[] buffer = new byte[1 << 20];
        long total = 0;
        try (FileOutputStream stream = new FileOutputStream(output)) {
            int count;
            while ((count = input.read(buffer)) != -1) {
                total += count;
                if (total > limit) throw new IOException("Wallet file exceeds its size limit: " + output.getName());
                if (dataDir.getUsableSpace() < RESERVE + count) throw new IOException("Free more device storage before importing wallet files");
                stream.write(buffer, 0, count);
            }
            if (total == 0) throw new IOException("Empty wallet file: " + output.getName());
            stream.getFD().sync();
        }
    }

    private static void verify(File stage, String prefix, String manifestName) throws Exception {
        byte[] encoded;
        try (InputStream input = new FileInputStream(new File(stage, manifestName));
             ByteArrayOutputStream output = new ByteArrayOutputStream()) {
            byte[] buffer = new byte[4096];
            int count;
            while ((count = input.read(buffer)) != -1) {
                if (output.size() + count > 4096) throw new IOException("Wallet manifest exceeds limit");
                output.write(buffer, 0, count);
            }
            encoded = output.toByteArray();
        }
        JSONObject manifest = new JSONObject(new String(encoded, StandardCharsets.UTF_8));
        Object version = manifest.get("version");
        if (manifest.length() != 4 || !(version instanceof Number) || ((Number) version).doubleValue() != 2) {
            throw new IOException("Unsupported wallet manifest version");
        }
        String[] extensions = {"r1cs", "pk", "vk"};
        String[] pins = {"r1csSHA256", "provingKeySHA256", "verifyingKeySHA256"};
        for (int i = 0; i < extensions.length; i++) {
            JSONArray values = manifest.getJSONArray(pins[i]);
            if (values.length() != 32) throw new IOException("Invalid wallet file hash");
            byte[] expected = new byte[32];
            for (int j = 0; j < expected.length; j++) {
                Object value = values.get(j);
                if (!(value instanceof Number)) throw new IOException("Invalid wallet hash byte");
                int integer = ((Number) value).intValue();
                if (integer < 0 || integer > 255 || ((Number) value).doubleValue() != integer) throw new IOException("Invalid wallet hash byte");
                expected[j] = (byte) integer;
            }
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            try (InputStream input = new FileInputStream(new File(stage, prefix + "." + extensions[i]))) {
                byte[] buffer = new byte[1 << 20];
                int count;
                while ((count = input.read(buffer)) != -1) digest.update(buffer, 0, count);
            }
            if (!MessageDigest.isEqual(expected, digest.digest())) throw new IOException("Wallet file hash mismatch: " + prefix + "." + extensions[i]);
        }
    }

    private static void remove(File file) {
        File[] children = file.listFiles();
        if (children != null) for (File child : children) remove(child);
        file.delete();
    }
}
