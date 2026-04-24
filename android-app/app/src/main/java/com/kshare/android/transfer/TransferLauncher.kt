package com.kshare.android.transfer

import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.core.content.ContextCompat
import com.kshare.android.FileTransferService
import java.util.ArrayList

object TransferLauncher {
    fun upload(context: Context, serverIp: String, serverPort: Int, pairingCode: String, uri: Uri) {
        val intent = baseIntent(context, FileTransferService.ACTION_UPLOAD, serverIp, serverPort, pairingCode)
            .apply {
                putExtra(FileTransferService.EXTRA_URI, uri.toString())
                data = uri
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            }
        ContextCompat.startForegroundService(context, intent)
    }

    fun upload(context: Context, serverIp: String, serverPort: Int, pairingCode: String, uris: List<Uri>) {
        if (uris.isEmpty()) return
        val intent = baseIntent(context, FileTransferService.ACTION_UPLOAD, serverIp, serverPort, pairingCode)
            .apply {
                putParcelableArrayListExtra(FileTransferService.EXTRA_URIS, ArrayList(uris))
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            }
        ContextCompat.startForegroundService(context, intent)
    }

    fun uploadFolder(context: Context, serverIp: String, serverPort: Int, pairingCode: String, treeUri: Uri) {
        val intent = baseIntent(context, FileTransferService.ACTION_UPLOAD_FOLDER, serverIp, serverPort, pairingCode)
            .apply {
                putExtra(FileTransferService.EXTRA_TREE_URI, treeUri.toString())
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            }
        ContextCompat.startForegroundService(context, intent)
    }

    fun download(context: Context, serverIp: String, serverPort: Int, pairingCode: String, fileName: String, isDir: Boolean) {
        val intent = baseIntent(context, FileTransferService.ACTION_DOWNLOAD, serverIp, serverPort, pairingCode)
            .apply {
                putExtra(FileTransferService.EXTRA_FILE_NAME, fileName)
                putExtra(FileTransferService.EXTRA_IS_DIR, isDir)
            }
        ContextCompat.startForegroundService(context, intent)
    }

    private fun baseIntent(
        context: Context,
        action: String,
        serverIp: String,
        serverPort: Int,
        pairingCode: String
    ): Intent {
        return Intent(context, FileTransferService::class.java).apply {
            this.action = action
            putExtra(FileTransferService.EXTRA_SERVER_IP, serverIp)
            putExtra(FileTransferService.EXTRA_SERVER_PORT, serverPort)
            putExtra(FileTransferService.EXTRA_PAIRING_CODE, pairingCode)
        }
    }
}
