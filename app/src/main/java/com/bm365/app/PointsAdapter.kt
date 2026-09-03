package com.bm365.app

import android.view.LayoutInflater
import android.view.ViewGroup
import androidx.recyclerview.widget.DiffUtil
import androidx.recyclerview.widget.ListAdapter
import androidx.recyclerview.widget.RecyclerView
import com.bm365.app.databinding.ItemPointsBinding

/**
 * 积分列表 RecyclerView 适配器
 */
class PointsAdapter : ListAdapter<PointsItem, PointsAdapter.ViewHolder>(DiffCallback) {

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): ViewHolder {
        val binding = ItemPointsBinding.inflate(
            LayoutInflater.from(parent.context), parent, false
        )
        return ViewHolder(binding)
    }

    override fun onBindViewHolder(holder: ViewHolder, position: Int) {
        holder.bind(getItem(position))
    }

    class ViewHolder(private val binding: ItemPointsBinding) :
        RecyclerView.ViewHolder(binding.root) {

        fun bind(item: PointsItem) {
            binding.taskName.text = item.name
            binding.currentPoints.text = item.current.toString()
            binding.maxPoints.text = "/${item.max}分"
            binding.taskProgress.progress = item.progressPercent
        }
    }

    companion object DiffCallback : DiffUtil.ItemCallback<PointsItem>() {
        override fun areItemsTheSame(oldItem: PointsItem, newItem: PointsItem): Boolean {
            return oldItem.name == newItem.name
        }

        override fun areContentsTheSame(oldItem: PointsItem, newItem: PointsItem): Boolean {
            return oldItem == newItem
        }
    }
}
