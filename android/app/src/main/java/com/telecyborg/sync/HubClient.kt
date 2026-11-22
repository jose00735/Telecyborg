package com.telecyborg.sync

import com.telecyborg.data.TcJson
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL
import java.net.URLEncoder
import java.security.MessageDigest
import java.security.SecureRandom
import java.security.cert.CertificateException
import java.security.cert.X509Certificate
import javax.net.ssl.HttpsURLConnection
import javax.net.ssl.SSLContext
import javax.net.ssl.X509TrustManager

/** One manifest line from the PC hub (Go: hub.Entry). */
@Serializable
data class Entry(val kind: String, val id: String, val hash: String = "", val modified: Long = 0, val deleted: Boolean = false)

@Serializable
data class Item(val kind: String = "", val id: String = "", val hash: String = "", val modified: Long = 0, val deleted: Boolean = false, val content: String = "")

class HubException(val code: Int, message: String, val current: Item? = null) : IOException(message)

/** The paired PC (stored in sync/state.json). */
@Serializable
data class PcLink(val host: String, val port: Int, val name: String, val fp: String, val phoneId: String, val token: String)

/**
 * HTTPS client to the TeleCyborg PC hub (internal/hub). The PC certificate is self-signed:
 * trust comes from pairing (the 6-digit code is bound to the certificate fingerprint) and from
 * pinning that fingerprint afterwards. Plain JVM code, so the JVM tests can run it against Go.
 */
class HubClient(private val host: String, private val port: Int, private val pinnedFp: String?, private val phoneId: String = "", private val token: String = "") {
    /** SHA-256 of the certificate the server presented on the last request (hex). */
    @Volatile var seenFp: String = ""
        private set

    private val ssl: SSLContext by lazy {
        // Not a CA check on purpose: see the class comment.
        @Suppress("CustomX509TrustManager")
        val tm = object : X509TrustManager {
            override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) = throw CertificateException()
            override fun checkServerTrusted(chain: Array<out X509Certificate>, authType: String?) {
                val fp = hex(MessageDigest.getInstance("SHA-256").digest(chain[0].encoded))
                seenFp = fp
                if (pinnedFp != null && pinnedFp != fp) throw CertificateException("PC certificate changed")
            }
            override fun getAcceptedIssuers(): Array<X509Certificate> = arrayOf()
        }
        SSLContext.getInstance("TLSv1.3").apply { init(null, arrayOf(tm), SecureRandom()) }
    }

    private fun request(method: String, path: String, body: String? = null, timeoutMs: Int = 15000): Pair<Int, String> {
        val h = if (host.contains(':')) "[$host]" else host
        val c = URL("https://$h:$port$path").openConnection() as HttpsURLConnection
        c.sslSocketFactory = ssl.socketFactory
        c.hostnameVerifier = javax.net.ssl.HostnameVerifier { _, _ -> true } // pinned by fingerprint instead
        c.requestMethod = method
        c.connectTimeout = 5000
        c.readTimeout = timeoutMs
        if (phoneId.isNotEmpty()) c.setRequestProperty("X-TC-Phone", phoneId)
        if (token.isNotEmpty()) c.setRequestProperty("Authorization", "Bearer $token")
        if (body != null) {
            c.doOutput = true
            c.setRequestProperty("Content-Type", "application/json")
            c.outputStream.use { it.write(body.toByteArray()) }
        }
        val code = c.responseCode
        val text = (if (code >= 400) c.errorStream else c.inputStream)?.use { it.readBytes().decodeToString() } ?: ""
        c.disconnect()
        return code to text
    }

    private fun obj(text: String): JsonObject = runCatching { TcJson.parseToJsonElement(text).jsonObject }.getOrDefault(JsonObject(emptyMap()))
    private fun err(code: Int, text: String): HubException {
        val o = obj(text)
        val cur = o["current"]?.let { runCatching { TcJson.decodeFromJsonElement(Item.serializer(), it) }.getOrNull() }
        val msg = (o["error"]?.toString()?.trim('"')) ?: "PC answered $code"
        return HubException(code, msg, cur)
    }

    fun hello(): JsonObject {
        val (code, text) = request("GET", "/v1/hello")
        if (code != 200) throw err(code, text)
        return obj(text)
    }

    // ---------- pairing (numeric comparison with commitment) ----------
    data class PairStart(val pairId: String, val commit: String, val pcName: String)

    fun pairBegin(phoneId: String, name: String): PairStart {
        val (code, text) = request("POST", "/v1/pair/begin", buildJsonObject { put("phone_id", phoneId); put("name", name) }.toString())
        if (code != 200) throw err(code, text)
        val o = obj(text)
        return PairStart(o.s("pair_id"), o.s("commit"), o.s("pc_name"))
    }

    /** Sends our nonce, checks the PC's revealed nonce against its commitment, returns the code. */
    fun pairNonce(start: PairStart, phoneNonce: ByteArray): String {
        val (code, text) = request("POST", "/v1/pair/nonce", buildJsonObject { put("pair_id", start.pairId); put("nonce", hex(phoneNonce)) }.toString())
        if (code != 200) throw err(code, text)
        val pcNonce = unhex(obj(text).s("nonce"))
        if (commit(pcNonce) != start.commit) throw IOException("The PC did not keep its commitment: do not pair")
        return sas(unhex(seenFp), pcNonce, phoneNonce)
    }

    /** Waits for the PC user; returns the token, or null when someone said no. */
    fun pairConfirm(start: PairStart, ok: Boolean): String? {
        val (code, text) = request("POST", "/v1/pair/confirm", buildJsonObject { put("pair_id", start.pairId); put("ok", ok) }.toString(), timeoutMs = 150_000)
        if (code != 200) throw err(code, text)
        val o = obj(text)
        return if (o["ok"]?.toString() == "true") o.s("token") else null
    }

    // ---------- sync ----------
    fun manifest(): List<Entry> {
        val (code, text) = request("GET", "/v1/manifest")
        if (code != 200) throw err(code, text)
        val items = obj(text)["items"] ?: return emptyList()
        return TcJson.decodeFromJsonElement(kotlinx.serialization.builtins.ListSerializer(Entry.serializer()), items)
    }

    private fun q(s: String) = URLEncoder.encode(s, "UTF-8")

    fun get(kind: String, id: String): Item {
        val (code, text) = request("GET", "/v1/item?kind=${q(kind)}&id=${q(id)}")
        if (code != 200) throw err(code, text)
        return TcJson.decodeFromString(Item.serializer(), text)
    }

    /** Stores an item on the PC; throws HubException(409, current) when the PC changed it. */
    fun put(kind: String, id: String, base: String, content: String): Item {
        val body = buildJsonObject { put("Kind", kind); put("ID", id); put("Base", base); put("Content", content) }.toString()
        val (code, text) = request("PUT", "/v1/item", body)
        if (code != 200) throw err(code, text)
        return TcJson.decodeFromString(Item.serializer(), text)
    }

    fun delete(kind: String, id: String, base: String) {
        val (code, text) = request("DELETE", "/v1/item?kind=${q(kind)}&id=${q(id)}&base=${q(base)}")
        if (code != 200) throw err(code, text)
    }

    companion object {
        fun hex(b: ByteArray) = b.joinToString("") { "%02x".format(it) }
        fun unhex(s: String): ByteArray = ByteArray(s.length / 2) { s.substring(it * 2, it * 2 + 2).toInt(16).toByte() }
        fun commit(nonce: ByteArray) = hex(MessageDigest.getInstance("SHA-256").digest(nonce))

        /** Same as Go's hub.SAS. */
        fun sas(certFp: ByteArray, pcNonce: ByteArray, phoneNonce: ByteArray): String {
            val md = MessageDigest.getInstance("SHA-256")
            md.update("telecyborg-pair-v1".toByteArray()); md.update(certFp); md.update(pcNonce); md.update(phoneNonce)
            val d = md.digest()
            val v = ((d[0].toLong() and 0xff) shl 24) or ((d[1].toLong() and 0xff) shl 16) or ((d[2].toLong() and 0xff) shl 8) or (d[3].toLong() and 0xff)
            return "%06d".format(v % 1_000_000)
        }
    }
}

private fun JsonObject.s(k: String) = this[k]?.toString()?.trim('"') ?: ""
