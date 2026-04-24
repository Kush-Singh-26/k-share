package com.kshare.android.feature.history

import com.kshare.android.api.HistoryItem

data class HistoryUiState(
    val items: List<HistoryItem> = emptyList(),
    val isVisible: Boolean = false,
    val isLoading: Boolean = false
)
