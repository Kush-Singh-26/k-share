package com.kshare.android.feature.history

import android.app.Application
import android.widget.Toast
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.kshare.android.api.HistoryItem
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

class HistoryViewModel(application: Application) : AndroidViewModel(application) {
    private val appContext = application.applicationContext
    private val controller = HistoryController()

    private val _uiState = MutableStateFlow(HistoryUiState())
    val uiState: StateFlow<HistoryUiState> = _uiState.asStateFlow()

    fun loadHistory(serverIp: String, port: Int, pairingCode: String) {
        if (serverIp.isBlank()) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true) }
            val items = controller.load(serverIp, port, pairingCode)
            if (items.isEmpty()) {
                _uiState.update { it.copy(items = emptyList(), isLoading = false, isVisible = false) }
                Toast.makeText(appContext, "No history", Toast.LENGTH_SHORT).show()
            } else {
                _uiState.update {
                    it.copy(
                        items = items,
                        isLoading = false,
                        isVisible = true
                    )
                }
            }
        }
    }

    fun deleteHistory(serverIp: String, port: Int, pairingCode: String, item: HistoryItem) {
        if (serverIp.isBlank()) return
        viewModelScope.launch {
            val deleted = controller.delete(serverIp, port, item.id, pairingCode)
            if (deleted) {
                _uiState.update { state ->
                    val remaining = state.items.filterNot { it.id == item.id }
                    state.copy(
                        items = remaining,
                        isVisible = remaining.isNotEmpty()
                    )
                }
            }
        }
    }

    fun dismiss() {
        _uiState.update { it.copy(isVisible = false) }
    }
}
